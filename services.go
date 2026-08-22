package main

import "os/exec"

func ServiceStatus() map[string]string { result := map[string]string{}; for _, service := range []string{"apache2", "httpd", "mysql", "mariadb", "php-fpm"} { if _, err := exec.LookPath(service); err == nil { result[service] = "installed" } }; return result }
