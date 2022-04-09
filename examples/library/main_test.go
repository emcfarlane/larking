package main

import (
	"testing"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/library/apipb"
	"github.com/emcfarlane/larking/starlib"
	"go.starlark.net/starlark"
)

/*func TestDynamicSet(t *testing.T) {
	d, err := protoregistry.GlobalFiles.FindDescriptorByName("larking.examples.library.Book")
	if err != nil {
		t.Fatal(err)
	}
	md := d.(protoreflect.MessageDescriptor)
	msg := dynamicpb.NewMessage(md)

	req := &apipb.CreateBookRequest{
		Parent: "/shelves/one",
	}
	x := req.ProtoReflect()
	rd := x.Descriptor()
	fd := rd.Fields().ByName("book")

	v := protoreflect.ValueOf(msg)
	x.Set(fd, v)

	t.Log("x", x)
}

func TestDynamicMessageType(t *testing.T) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName("larking.examples.library.Book")
	if err != nil {
		t.Fatal(err)
	}
	msg := mt.New()

	req := &apipb.CreateBookRequest{
		Parent: "/shelves/one",
	}
	x := req.ProtoReflect()
	rd := x.Descriptor()
	fd := rd.Fields().ByName("book")

	v := protoreflect.ValueOf(msg)
	x.Set(fd, v)

	t.Log("x", x)
}*/

func TestScripts(t *testing.T) {
	s := &Server{}

	mux, err := larking.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	mux.RegisterService(&apipb.Library_ServiceDesc, s)

	starlib.RunTests(t, "testdata/*.star", starlark.StringDict{
		"mux": mux,
	})
}
