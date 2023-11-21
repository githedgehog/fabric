package wiring

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func init() {
	scheme := runtime.NewScheme()
	if err := wiringapi.AddToScheme(scheme); err != nil {
		log.Fatalf("error adding fabricv1alpha1 to the scheme: %#v", err)
	}
	if err := vpcapi.AddToScheme(scheme); err != nil {
		log.Fatalf("error adding vpcv1alpha1 to the scheme: %#v", err)
	}

	decoder = serializer.NewCodecFactory(scheme).UniversalDeserializer()
}

var decoder runtime.Decoder

// TODO report list of files/sources
func LoadDataFrom(from string) (*Data, error) {
	data, err := New()
	if err != nil {
		return nil, err
	}

	if from == "-" {
		return data, errors.Wrap(Load(os.Stdin, data), "error loading from stdin")
	}

	fromFile := "."

	if info, err := os.Stat(from); err == nil && !info.IsDir() {
		fromFile = filepath.Base(from)
		from = filepath.Dir(from)
	}

	// log.Println("Loading data from directory (recursively)", from)
	f := os.DirFS(from)
	err = LoadDir(f, fromFile, data)
	if err != nil {
		return nil, errors.Wrap(err, "error loading dir")
	}

	return data, nil
}

func LoadDir(f fs.FS, root string, data *Data) error {
	err := fs.WalkDir(f, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// log.Println("Walking into", path)

			return nil
		}

		if filepath.Ext(path) != ".yaml" || strings.Contains(path, "kustom") || strings.Contains(path, ".skip.") {
			// log.Println("Skipping file", path)

			return nil
		}

		// log.Println("Loading data from", path)

		err = LoadFile(f, path, data)
		if err != nil {
			return errors.Wrapf(err, "error loading file %s", path)
		}

		return nil
	})

	return err
}

func LoadFile(f fs.FS, path string, data *Data) error {
	yamlFile, err := fs.ReadFile(f, path)
	if err != nil {
		return errors.Wrapf(err, "error reading file %s", path)
	}

	return errors.Wrapf(Load(bytes.NewReader(yamlFile), data), "error loading file %s", path)
}

func Load(r io.Reader, data *Data) error {
	multidocReader := utilyaml.NewYAMLReader(bufio.NewReader(r))

	for {
		buf, err := multidocReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "error multidoc-parsing")
		}

		obj, _, err := decoder.Decode(buf, nil, nil)
		if err != nil {
			return errors.Wrap(err, "error decoding object")
		}

		switch typed := obj.(type) {
		case *wiringapi.Rack:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.Switch:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.Server:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.Connection:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.SwitchProfile:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.ServerProfile:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *vpcapi.IPv4Namespace:
			if err := data.Add(typed); err != nil {
				return err
			}
		case *wiringapi.VLANNamespace:
			if err := data.Add(typed); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Data) SaveTo(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "error creating file %s", path)
	}
	defer f.Close()

	return errors.Wrapf(d.Write(f), "error saving to file %s", path)
}
