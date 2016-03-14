package compiler

import (
	"fmt"

	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshsys "github.com/cloudfoundry/bosh-utils/system"
)

func (c concreteCompiler) RunPackagingCommand(compilePath, enablePath string, pkg Package) error {
	runCommand := fmt.Sprintf("iex ((get-content %s) -join \"`n\")", PackagingScriptName)
	command := boshsys.Command{
		Name: "powershell",
		Args: []string{"-NoProfile", "-NonInteractive", "-command", runCommand},
		Env: map[string]string{
			"BOSH_COMPILE_TARGET":  compilePath,
			"BOSH_INSTALL_TARGET":  enablePath,
			"BOSH_PACKAGE_NAME":    pkg.Name,
			"BOSH_PACKAGE_VERSION": pkg.Version,
		},
		WorkingDir: compilePath,
	}

	_, err := c.runner.RunCommand("compilation", PackagingScriptName, command)
	if err != nil {
		return bosherr.WrapError(err, "Running packaging script")
	}
	return nil
}
