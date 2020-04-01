package main

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TODO: this is only for GRPC hacking
var mappings = map[string]string{
	"asdfasdf": "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody",
	"google.golang.org/genproto/googleapis/rpc/codes":  "github.com/afking/graphpb/google.golang.org/genproto/googleapis/rpc/codes",
	"google.golang.org/genproto/googleapis/rpc/status": "github.com/afking/graphpb/google.golang.org/genproto/googleapis/rpc/status",
	//"src/google/protobuf/any.proto":                  "google.golang.org/protobuf/types/known/anypb",
	//"src/google/protobuf/api.proto":                  "google.golang.org/protobuf/types/known/apipb",
	"github.com/golang/protobuf/ptypes/duration": "google.golang.org/protobuf/types/known/durationpb",
	"github.com/golang/protobuf/ptypes/empty":    "google.golang.org/protobuf/types/known/emptypb",
	//"src/google/protobuf/field_mask.proto":           "google.golang.org/protobuf/types/known/fieldmaskpb",
	//"src/google/protobuf/source_context.proto":       "google.golang.org/protobuf/types/known/sourcecontextpb",
	//"src/google/protobuf/struct.proto":               "google.golang.org/protobuf/types/known/structpb",
	"github.com/golang/protobuf/ptypes/timestamp": "google.golang.org/protobuf/types/known/timestamppb",
	//"src/google/protobuf/type.proto":                 "google.golang.org/protobuf/types/known/typepb",
	//"github.com/golang/protobuf/ptypes/wrappers":             "google.golang.org/protobuf/types/known/wrapperspb",
	//"src/google/protobuf/descriptor.proto":           "google.golang.org/protobuf/types/descriptorpb",
	"google.golang.org/grpc/codes":  "github.com/afking/graphpb/grpc/codes",
	"google.golang.org/grpc/status": "github.com/afking/graphpb/grpc/status",

	"google.golang.org/genproto/googleapis/api/annotations": "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations",
}

func run() error {
	fs, err := filepath.Glob("*.go")
	if err != nil {
		return err
	}

	for _, name := range fs {
		b, err := ioutil.ReadFile(name)
		if err != nil {
			return err
		}

		s := string(b)
		for from, to := range mappings {
			s = strings.Replace(s, strconv.Quote(from), strconv.Quote(to), 1)
		}

		f, err := os.Create(name)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(f, strings.NewReader(s)); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
