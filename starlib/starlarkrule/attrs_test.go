package starlarkrule

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"larking.io/api/actionpb"
)

func TestProtoTypes(t *testing.T) {

	for _, m := range []proto.Message{
		&wrapperspb.StringValue{Value: "Hello, world"},
		&actionpb.LabelValue{Value: "Label"},
	} {
		t.Log(m)

		r := m.ProtoReflect()
		t.Log(r.Type())

		any, err := anypb.New(m)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(any.TypeUrl)

	}

}
