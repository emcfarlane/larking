package main

import (
	"context"
	"os"
	"testing"
)

func TestBoot(t *testing.T) {
	content := []byte("1+1\n")
	tmpin, err := os.CreateTemp("", "bootin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpin.Name()) // clean up
	defer tmpin.Close()

	if _, err := tmpin.Write(content); err != nil {
		t.Fatal(err)
	}

	if _, err := tmpin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }() // Restore original Stdin
	os.Stdin = tmpin

	tmpout, err := os.CreateTemp("", "bootout")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpout.Name()) // clean up
	defer tmpout.Close()

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }() // Restore original Stdout
	os.Stdout = tmpout

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := &Options{}
	if err := run(ctx, opts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(tmpout.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + string(b))
}

func TestBootFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := &Options{
		Filename: "<stdin>",
		Source:   "a = 1+1",
	}
	if err := exec(ctx, opts); err != nil {
		t.Fatal(err)
	}
}
