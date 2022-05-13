package starlarkrule

import (
	"fmt"
	"os"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	cname "github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"go.starlark.net/starlark"
)

// conatiner rules implemented with go-containerregistry.
// Based on:
// https://github.com/google/ko/blob/main/pkg/build/gobuild.go
// https://github.com/bazelbuild/rules_docker/tree/master/container
var containerModule = &starlarkstruct.Module{
	Name: "container",
	Members: starlark.StringDict{
		"pull":  starext.MakeBuiltin("container.pull", containerPull),
		"build": starext.MakeBuiltin("container.build", containerBuild),
		"push":  starext.MakeBuiltin("container.push", containerPush),
	},
}

const ImageConstructor starlark.String = "container.image"

// TODO: return starlark provider.
func NewContainerImage(filename, reference string) starlark.Value {
	return starlarkstruct.FromStringDict(ImageConstructor, map[string]starlark.Value{
		"name":      starlark.String(filename),
		"reference": starlark.String(reference),
	})
}

func containerPull(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		rname     string
		reference string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"name", &rname, "reference", &reference,
	); err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(reference)
	if err != nil {
		return nil, err
	}
	ref.Context()

	ctx := starlarkthread.GetContext(thread)

	img, err := remote.Image(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}

	// TODO: caching...
	// HACK: lets just stat the existance of the file
	filename := "" // c.key
	if _, err := os.Stat(filename); err != nil {
		f, err := os.Create(filename)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if err := tarball.Write(ref, img, f); err != nil {
			return nil, err
		}
	}
	return NewContainerImage(filename, reference), nil
}

func listToStrings(l *starlark.List) ([]string, error) {
	iter := l.Iterate()
	defer iter.Done()

	var ss []string

	var x starlark.Value
	for iter.Next(&x) {
		s, ok := starlark.AsString(x)
		if !ok {
			return nil, fmt.Errorf("invalid string list")
		}
		ss = append(ss, s)
	}
	return ss, nil
}

func containerBuild(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name            string
		entrypointList  *starlark.List
		tar             *Label
		base            *Label
		prioritizedList *starlark.List
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"name", &name,
		"entrypoint", &entrypointList,
		"tar", &tar,
		"base?", &base,
		"prioritized_files?", &prioritizedList,
	); err != nil {
		return nil, err
	}

	// TODO: load tag from provider?
	entrypoint, err := listToStrings(entrypointList)
	if err != nil {
		return nil, err
	}
	prioritizedFiles, err := listToStrings(prioritizedList)
	if err != nil {
		return nil, err
	}

	baseImage := empty.Image
	if base != nil {
		// Load base image from local.
		imageProvider, err := toStruct(base, ImageConstructor)
		if err != nil {
			return nil, fmt.Errorf("image provider: %w", err)
		}
		if err := assertConstructor(imageProvider, ImageConstructor); err != nil {
			return nil, err
		}

		filename, err := getAttrStr(imageProvider, "name")
		if err != nil {
			return nil, err
		}

		reference, err := getAttrStr(imageProvider, "reference")
		if err != nil {
			return nil, err
		}

		tag, err := cname.NewTag(reference, cname.StrictValidation)
		if err != nil {
			return nil, err
		}

		// Load base from filesystem.
		img, err := tarball.ImageFromPath(filename, &tag)
		if err != nil {
			return nil, err
		}
		baseImage = img
	}

	var layers []mutate.Addendum

	tarStruct, err := toStruct(tar, FileConstructor)
	if err != nil {
		return nil, err
	}
	tarFilename, err := getAttrStr(tarStruct, "path")
	if err != nil {
		return nil, err
	}

	r, err := os.Open(tarFilename)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	imageLayer, err := tarball.LayerFromReader(
		r, tarball.WithCompressedCaching,
		tarball.WithEstargzOptions(estargz.WithPrioritizedFiles(
			// When using estargz, prioritize downloading the binary entrypoint.
			prioritizedFiles,
		)),
	)
	if err != nil {
		return nil, err
	}
	layers = append(layers, mutate.Addendum{
		Layer: imageLayer,
		History: v1.History{
			Author:    "laze",
			CreatedBy: "laze " + name,
			Comment:   "ship it real good",
		},
	})

	// Augment the base image with our application layer.
	appImage, err := mutate.Append(baseImage, layers...)
	if err != nil {
		return nil, err
	}

	// Start from a copy of the base image's config file, and set
	// the entrypoint to our app.
	cfg, err := appImage.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg = cfg.DeepCopy()
	cfg.Config.Entrypoint = entrypoint
	//updatePath(cfg)
	cfg.Config.Env = append(cfg.Config.Env, "LAZE_DATA_PATH="+"/") // TODO
	cfg.Author = "github.com/emcfarlane/laze"

	if cfg.Config.Labels == nil {
		cfg.Config.Labels = map[string]string{}
	}
	// TODO: Add support for labels.
	//for k, v := range labels {
	//	cfg.Config.Labels[k] = v
	//}

	img, err := mutate.ConfigFile(appImage, cfg)
	if err != nil {
		return nil, err
	}

	//empty := v1.Time{}
	//if g.creationTime != empty {
	//	return mutate.CreatedAt(image, g.creationTime)
	//}

	filename := "" // c.key
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	reference := "gcr.io/foo/bar:latest" // TODO: Reference?
	ref, err := cname.ParseReference(reference)
	if err != nil {
		return nil, err
	}

	if err := tarball.Write(ref, img, f); err != nil {
		return nil, err
	}
	return NewContainerImage(filename, reference), nil
}

func containerPush(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	fmt.Println("RUNNING CONTAINER PUSH")
	ctx := starlarkthread.GetContext(thread)
	var (
		name      string
		image     *Label
		reference string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"name", &name,
		"image", &image,
		"reference", &reference,
	); err != nil {
		fmt.Println("failed on starlark")
		return nil, err
	}

	imageProvider, err := toStruct(image, ImageConstructor)
	if err != nil {
		return nil, fmt.Errorf("image provider: %w", err)
	}

	// TODO: should it be a file provider?
	filename, err := getAttrStr(imageProvider, "name")
	if err != nil {
		return nil, err
	}
	imageRef, err := getAttrStr(imageProvider, "reference")
	if err != nil {
		return nil, err
	}

	tag, err := cname.NewTag(imageRef, cname.StrictValidation)
	if err != nil {
		fmt.Println("FAILED ON tag")
		return nil, err
	}

	// Load base from filesystem.
	img, err := tarball.ImageFromPath(filename, &tag)
	if err != nil {
		fmt.Println("FAILED ON image load")
		return nil, err
	}

	ref, err := cname.ParseReference(reference)
	if err != nil {
		fmt.Println("FAILED ON REF")
		return nil, err
	}

	if err := remote.Write(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	); err != nil {
		fmt.Println("failing here?", err)
		return nil, err
	}
	fmt.Println("DONE CONTAINER PUSH")
	return NewContainerImage(filename, reference), nil
}
