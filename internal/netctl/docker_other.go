//go:build !linux

package netctl

import "fmt"

func EnableDockerTCPRedirect(redirPort int) error {
	return fmt.Errorf("docker redirect is supported on Linux only")
}

func DisableDockerTCPRedirect() error {
	return fmt.Errorf("docker redirect is supported on Linux only")
}

func DockerTCPRedirectStatus() (bool, error) {
	return false, fmt.Errorf("docker redirect is supported on Linux only")
}
