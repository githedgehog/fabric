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
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func LoadDataFrom(from string, data *Data) error {
	var err error
	if data == nil {
		data, err = New()
		if err != nil {
			return err
		}

	}

	if from == "-" {
		return errors.Wrap(Load(os.Stdin, data), "error loading from stdin")
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
		return errors.Wrap(err, "error loading dir")
	}

	return nil
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
		metaObj, ok := obj.(metav1.Object)
		if !ok {
			return errors.Errorf("object %#v is not a metav1.Object", obj)
		}

		group := obj.GetObjectKind().GroupVersionKind().Group
		if group != wiringapi.GroupVersion.Group && group != vpcapi.GroupVersion.Group {
			return errors.Errorf("object has unknown or unsupported group %s", group)
		}

		if fabricObj, ok := obj.(meta.Object); !ok {
			return errors.Errorf("object %#v is not a Fabric Object", obj)
		} else {
			fabricObj.Default()
		}

		if err := data.Add(metaObj); err != nil {
			return err
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
