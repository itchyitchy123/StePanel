package main

import (
	"bytes"
	"os/exec"
)

func ServiceStatus() map[string]string {
	result := map[string]string{}
	for _, service := range []string{"apache2", "httpd", "mysql", "mariadb", "php-fpm", "fail2ban", "fpm-lens"} {
		if _, err := exec.LookPath(service); err == nil {
			result[service] = "installed"
		}
	}
	for _, apache := range []string{"apachectl", "httpd"} {
		if _, err := exec.LookPath(apache); err != nil {
			continue
		}
		if output, err := exec.Command(apache, "-M").CombinedOutput(); err == nil && (bytes.Contains(output, []byte("security2_module")) || bytes.Contains(output, []byte("security3_module"))) {
			result["modsecurity"] = "enabled"
			break
		}
	}
	return result
}
