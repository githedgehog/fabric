package common

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

func LoadCtrlConfig(basedir, name string, cfg any) error {
	path := filepath.Join(basedir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "error reading config %s", path)
	}

	// TODO log
	fmt.Println(name)
	fmt.Println(string(data))

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config %s", path)
	}

	return nil
}
