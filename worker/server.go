// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/emcfarlane/starlarkassert"
	"larking.io/api/actionpb"
	"larking.io/api/controlpb"
	"larking.io/api/workerpb"
	"larking.io/starlib"
	"larking.io/starlib/encoding/starlarkproto"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkthread"
)

type LoaderFunc func(*starlark.Thread, string) (starlark.StringDict, error)

func (f LoaderFunc) Load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	return f(thread, module)
}

type Loader interface {
	Load(thread *starlark.Thread, module string) (starlark.StringDict, error)
}

type Server struct {
	workerpb.UnimplementedWorkerServer
	loader  Loader
	control controlpb.ControlClient
	name    string
}

func NewServer(
	loader Loader,
	control controlpb.ControlClient,
	name string,
) *Server {
	return &Server{
		loader:  loader,
		control: control,
		name:    name,
	}
}

func (s *Server) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if s.loader == nil {
		return nil, status.Error(
			codes.Unavailable,
			"module loading not avaliable",
		)
	}
	return s.loader.Load(thread, module)
}

func (s *Server) authorize(ctx context.Context, op *controlpb.Operation) error {
	req := &controlpb.CheckRequest{
		Name:      s.name,
		Operation: op,
	}

	rsp, err := s.control.Check(ctx, req)
	if err != nil {
		return err
	}
	if s := rsp.Status; s != nil {
		st := status.FromProto(s)
		return st.Err()
	}
	return nil
}

var (
	errMissingCredentials = status.Error(codes.Unauthenticated, "missing credentials")
	errInvalidCredentials = status.Error(codes.Unauthenticated, "invalid credentials")
)

func extractCredentials(ctx context.Context) (*controlpb.Credentials, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	for _, hdrKey := range []string{"http-authorization", "authorization"} {
		keys := md.Get(hdrKey)
		if len(keys) == 0 {
			continue
		}
		vals := strings.Split(keys[0], " ")
		if len(vals) == 1 && len(vals[0]) == 0 {
			continue
		}
		if len(vals) != 2 {
			return nil, errMissingCredentials
		}
		val := vals[1]

		switch strings.ToLower(vals[0]) {
		case "bearer":
			return &controlpb.Credentials{
				Type: &controlpb.Credentials_Bearer{
					Bearer: &controlpb.Credentials_BearerToken{
						AccessToken: val,
					},
				},
			}, nil

		case "basic":
			c, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return nil, err
			}
			cs := string(c)
			s := strings.IndexByte(cs, ':')
			if s < 0 {
				return nil, errMissingCredentials
			}

			return &controlpb.Credentials{
				Type: &controlpb.Credentials_Basic{
					Basic: &controlpb.Credentials_BasicAuth{
						Username: cs[:s],
						Password: cs[s+1:],
					},
				},
			}, nil

		default:
			return nil, errInvalidCredentials
		}
	}
	return &controlpb.Credentials{
		Type: &controlpb.Credentials_Insecure{
			Insecure: true,
		},
	}, nil
}

func soleExpr(f *syntax.File) syntax.Expr {
	if len(f.Stmts) == 1 {
		if stmt, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
			return stmt.X
		}
	}
	return nil
}

// Create ServerStream...
func (s *Server) RunOnThread(stream workerpb.Worker_RunOnThreadServer) (err error) {
	ctx := stream.Context()
	l := logr.FromContextOrDiscard(ctx)

	cmd, err := stream.Recv()
	if err != nil {
		return err
	}
	l.Info("running on thread", "thread", cmd.Name)

	creds, err := extractCredentials(ctx)
	if err != nil {
		return err
	}

	op := &controlpb.Operation{
		Name:        cmd.Name,
		Credentials: creds,
	}

	if err := s.authorize(ctx, op); err != nil {
		l.Error(err, "failed to authorize request", "name", cmd.Name)
		return err
	}

	name := strings.TrimPrefix(cmd.Name, "thread/")

	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: name,
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: s.load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := cleanup(); err == nil {
			err = cerr
		}
	}()

	globals := starlib.NewGlobals()
	if name != "" {
		predeclared, err := s.load(thread, name)
		if err != nil {
			return err
		}
		for key, val := range predeclared {
			globals[key] = val // copy thread values to globals
		}
		thread.Name = name
	}

	run := func(input string) error {
		buf.Reset()
		f, err := syntax.Parse(thread.Name, input, 0)
		if err != nil {
			return err
		}

		if expr := soleExpr(f); expr != nil {
			// eval
			v, err := starlark.EvalExpr(thread, expr, globals)
			if err != nil {
				return err
			}

			// print
			if v != starlark.None {
				buf.WriteString(v.String())
			}
		} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			return err
		}
		return nil
	}

	c := starlib.Completer{StringDict: globals}
	for {
		result := &workerpb.Result{}

		switch v := cmd.Exec.(type) {
		case *workerpb.Command_Input:
			err := run(v.Input)
			if err != nil {
				l.Info("thread error", "err", err)
			}
			result.Result = &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: buf.String(),
					Status: errorStatus(err).Proto(),
				},
			}

		case *workerpb.Command_Complete:
			completions := c.Complete(v.Complete)
			result.Result = &workerpb.Result_Completion{
				Completion: &workerpb.Completion{
					Completions: completions,
				},
			}

		case *workerpb.Command_Format:
			b, err := Format(ctx, name, v.Format)
			if err != nil {
				l.Info("thread format error", "err", err)
			}

			result.Result = &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: string(b),
					Status: errorStatus(err).Proto(),
				},
			}
		}
		if err = stream.Send(result); err != nil {
			return err
		}

		cmd, err = stream.Recv()
		if err != nil {
			return err
		}
	}
}

func (s *Server) RunThread(ctx context.Context, req *workerpb.RunThreadRequest) (*workerpb.Output, error) {

	l := logr.FromContextOrDiscard(ctx)
	l.Info("running thread", "thread", req.Name)

	creds, err := extractCredentials(ctx)
	if err != nil {
		return nil, err
	}
	op := &controlpb.Operation{
		Name:        req.Name,
		Credentials: creds,
	}
	if err := s.authorize(ctx, op); err != nil {
		l.Error(err, "failed to authorize request", "name", req.Name)
		return nil, err
	}

	name := strings.TrimPrefix(req.Name, "thread/")

	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: name,
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: s.load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := cleanup(); err == nil {
			err = cerr
		}
	}()

	if name == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"missing module name",
		)
	}
	if _, err := s.load(thread, name); err != nil {
		return nil, err
	}

	return &workerpb.Output{
		Output: buf.String(),
		Status: errorStatus(err).Proto(),
	}, nil

}
func (s *Server) TestThread(ctx context.Context, req *workerpb.TestThreadRequest) (*workerpb.Output, error) {
	l := logr.FromContextOrDiscard(ctx)
	l.Info("testing thread", "thread", req.Name)

	creds, err := extractCredentials(ctx)
	if err != nil {
		return nil, err
	}
	op := &controlpb.Operation{
		Name:        req.Name,
		Credentials: creds,
	}
	if err := s.authorize(ctx, op); err != nil {
		l.Error(err, "failed to authorize request", "name", req.Name)
		return nil, err
	}

	name := strings.TrimPrefix(req.Name, "thread/")

	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: name,
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: s.load,
	}
	values, err := s.load(thread, name)
	if err != nil {
		return nil, err
	}

	errorf := func(err error) {
		switch err := err.(type) {
		case *starlark.EvalError:
			var found bool
			for i := range err.CallStack {
				posn := err.CallStack.At(i).Pos
				if posn.Filename() == name {
					linenum := int(posn.Line)
					msg := err.Error()

					fmt.Fprintf(&buf, "\n%s:%d: unexpected error: %v", name, linenum, msg)
					found = true
					break
				}
			}
			if !found {
				fmt.Fprint(&buf, err.Backtrace()) //nolint
			}
		case nil:
			// success
		default:
			fmt.Fprintf(&buf, "\n%s", err) //nolint
		}
	}

	tests := []testing.InternalTest{{
		Name: name,
		F: func(t *testing.T) {
			for key, val := range values {
				if !strings.HasPrefix(key, "test_") {
					continue // ignore
				}
				if _, ok := val.(starlark.Callable); !ok {
					continue // ignore non callable
				}

				key, val := key, val
				t.Run(key, func(t *testing.T) {
					tt := starlarkassert.NewTest(t)
					if _, err := starlark.Call(
						thread, val, starlark.Tuple{tt}, nil,
					); err != nil {
						errorf(err)
					}
				})
			}

		},
	}}

	var (
		matchPat string
		matchRe  *regexp.Regexp
	)
	deps := starlarkassert.MatchStringOnly(
		func(pat, str string) (result bool, err error) {
			if matchRe == nil || matchPat != pat {
				matchPat = pat
				matchRe, err = regexp.Compile(matchPat)
				if err != nil {
					return
				}
			}
			return matchRe.MatchString(str), nil
		},
	)
	var result *status.Status
	if testing.MainStart(deps, tests, nil, nil, nil).Run() > 0 {
		result = status.New(
			codes.Unknown, // TODO: error code.
			"failed",
		)
	} else {
		result = status.New(
			codes.OK,
			"passed",
		)
	}

	return &workerpb.Output{
		Output: buf.String(),
		Status: result.Proto(),
	}, nil
}

func toStarlark(source *starlarkrule.Label, val *actionpb.Value) (starlark.Value, error) {
	switch v := val.Kind.(type) {
	case *actionpb.Value_LabelValue:
		l, err := source.Parse(v.LabelValue)
		if err != nil {
			return nil, err
		}
		return l, nil
	case *actionpb.Value_BoolValue:
		return starlark.Bool(v.BoolValue), nil
	case *actionpb.Value_IntValue:
		return starlark.MakeInt64(v.IntValue), nil
	case *actionpb.Value_FloatValue:
		return starlark.Float(v.FloatValue), nil
	case *actionpb.Value_StringValue:
		return starlark.String(v.StringValue), nil
	case *actionpb.Value_BytesValue:
		return starlark.Bytes(v.BytesValue), nil
	case *actionpb.Value_ListValue:
		elems := make([]starlark.Value, len(v.ListValue.Values))
		for i, x := range v.ListValue.Values {
			value, err := toStarlark(source, x)
			if err != nil {
				return nil, err
			}
			elems[i] = value
		}
		return starlark.NewList(elems), nil
	case *actionpb.Value_DictValue:
		n := len(v.DictValue.Pairs)
		dict := starlark.NewDict(n / 2)
		for i := 0; i < n; i += 2 {
			key, val := v.DictValue.Pairs[i], v.DictValue.Pairs[i+1]
			k, err := toStarlark(source, key)
			if err != nil {
				return nil, err
			}
			v, err := toStarlark(source, val)
			if err != nil {
				return nil, err
			}
			dict.SetKey(k, v)
		}
		return dict, nil
	case *actionpb.Value_Message:
		m, err := v.Message.UnmarshalNew()
		if err != nil {
			return nil, err
		}
		return starlarkproto.NewMessage(m.ProtoReflect(), nil, nil)
	default:
		return nil, fmt.Errorf("unknown proto type: %T", v)
	}
}

func toValue(value starlark.Value) (*actionpb.Value, error) {
	switch v := value.(type) {
	case *starlarkrule.Label:
		return &actionpb.Value{
			Kind: &actionpb.Value_LabelValue{LabelValue: v.Key()},
		}, nil
	case starlark.Bool:
		return &actionpb.Value{
			Kind: &actionpb.Value_BoolValue{BoolValue: bool(v)},
		}, nil
	case starlark.Int:
		x, _ := v.Int64()
		return &actionpb.Value{
			Kind: &actionpb.Value_IntValue{IntValue: x},
		}, nil
	case starlark.Float:
		return &actionpb.Value{
			Kind: &actionpb.Value_FloatValue{FloatValue: float64(v)},
		}, nil
	case starlark.String:
		return &actionpb.Value{
			Kind: &actionpb.Value_StringValue{StringValue: string(v)},
		}, nil
	case starlark.Bytes:
		return &actionpb.Value{
			Kind: &actionpb.Value_BytesValue{BytesValue: []byte(v)},
		}, nil
	case *starlark.List:
		n := v.Len()
		values := make([]*actionpb.Value, n)
		for i := 0; i < n; i++ {
			val, err := toValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			values[i] = val
		}

		return &actionpb.Value{
			Kind: &actionpb.Value_ListValue{ListValue: &actionpb.ListValue{
				Values: values,
			}},
		}, nil
	case *starlark.Dict:
		items := v.Items()
		pairs := make([]*actionpb.Value, 0, 2*len(items))
		for _, kv := range items {
			key, err := toValue(kv[0])
			if err != nil {
				return nil, err
			}
			val, err := toValue(kv[1])
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, key, val)
		}

		return &actionpb.Value{
			Kind: &actionpb.Value_DictValue{DictValue: &actionpb.DictValue{
				Pairs: pairs,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("unknown starlark type: %q", v.Type())
	}
}

func makeKwargs(source *starlarkrule.Label, attrs *starlarkrule.Attrs, kws map[string]*actionpb.Value) (*starlarkrule.Kwargs, error) {
	kwargs := make([]starlark.Tuple, 0, len(kws))
	for key, val := range kws {
		value, err := toStarlark(source, val)
		if err != nil {
			return nil, err
		}
		kwargs = append(kwargs, starlark.Tuple{
			starlark.String(key), value,
		})
	}

	return starlarkrule.NewKwargs(attrs, kwargs)
}

func (s *Server) ExecuteAction(ctx context.Context, req *workerpb.ExecuteActionRequest) (*workerpb.ExecuteActionResponse, error) {
	l := logr.FromContextOrDiscard(ctx)
	l.Info("testing thread", "thread", req.Name)

	creds, err := extractCredentials(ctx)
	if err != nil {
		return nil, err
	}
	op := &controlpb.Operation{
		Name:        req.Name,
		Credentials: creds,
	}
	if err := s.authorize(ctx, op); err != nil {
		l.Error(err, "failed to authorize request", "name", req.Name)
		return nil, err
	}

	name := req.Name
	label, err := starlarkrule.ParseRelativeLabel("file://./?metadata=skip", name)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: label.String(),
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: s.load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := cleanup(); err == nil {
			err = cerr
		}
	}()

	b, err := starlarkrule.NewBuilder(label)
	if err != nil {
		return nil, err
	}

	apb := req.Action

	var deps []*starlarkrule.Action
	for _, dpb := range apb.Deps {
		var values []starlarkrule.AttrValue
		for _, vpb := range dpb.Values {
			fmt.Println("vdp", vpb)
			values = append(values, starlarkrule.AttrValue{
				//
				Value: nil, // TODO: how to translate values???
			})
		}

		deps = append(deps, &starlarkrule.Action{
			Target: nil,
			Deps:   nil,

			Values:    values,
			Error:     status.ErrorProto(dpb.Status),
			Failed:    dpb.Failed,
			ReadyTime: dpb.ReadyTime.AsTime(),
			DoneTime:  dpb.DoneTime.AsTime(),
		})
	}

	module, err := s.load(thread, apb.Target.RuleModule)
	if err != nil {
		return nil, fmt.Errorf("failed to load module %q: %w", apb.Target.RuleModule, err)
	}
	ruleValue := module[apb.Target.RuleName]
	rule, ok := ruleValue.(*starlarkrule.Rule)
	if !ok {
		return nil, fmt.Errorf("invalid rulename type: %q", ruleValue)
	}

	kwargs, err := makeKwargs(label, rule.Attrs(), apb.Target.Kwargs)
	if err != nil {
		return nil, err
	}

	targetLabel, err := label.Parse(apb.Target.Name)
	if err != nil {
		return nil, err
	}

	target := &starlarkrule.Target{
		Label:  targetLabel,
		Rule:   rule,
		Kwargs: kwargs,
	}

	a := &starlarkrule.Action{
		Target: target,
		Deps:   deps,
	}

	b.RunAction(thread, a)

	apb.Values = make([]*actionpb.Value, len(a.Values))
	fmt.Println("values", a.Values)
	for i, v := range a.Values {
		fmt.Println(v.Value)
		val, err := toValue(v.Value)
		if err != nil {
			return nil, err
		}
		apb.Values[i] = val
	}

	if a.Error != nil {
		apb.Status = errorStatus(a.Error).Proto()
	}
	apb.ReadyTime = timestamppb.New(a.ReadyTime)
	apb.DoneTime = timestamppb.New(a.DoneTime)

	return &workerpb.ExecuteActionResponse{
		Action: apb,
	}, nil
}
