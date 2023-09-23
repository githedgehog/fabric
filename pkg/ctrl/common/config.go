package common

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

func LoadCtrlConfig(basedir, name string, cfg any) error {
	path := filepath.Join(basedir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "error reading config %s", path)
	}

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config %s", path)
	}

	slog.Debug("Loaded controller config", "name", name, "data", spew.Sdump(cfg))

	return nil
}
