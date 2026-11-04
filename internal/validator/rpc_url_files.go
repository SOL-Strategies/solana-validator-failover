package validator

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// resolveClusterRPCEndpoints resolves selected sources without modifying configuration.
func resolveClusterRPCEndpoints(cluster, singular string, sources []string) ([]string, map[string]string, error) {
	selected, err := clusterRPCSources(cluster, singular, sources)
	if err != nil {
		return nil, nil, err
	}
	endpoints := make([]string, 0, len(selected))
	labels := make(map[string]string)
	seen := make(map[string]bool)
	field := "validator.cluster_rpc_url"
	if len(sources) > 0 {
		field = "validator.cluster_rpc_urls"
	}
	for i, source := range selected {
		endpoint, label, err := resolveRPCURL(source)
		if err != nil {
			return nil, nil, fmt.Errorf("%s (selected endpoint %d): %w", field, i+1, err)
		}
		if label != "" && labels[endpoint] == "" {
			labels[endpoint] = label
		}
		if !seen[endpoint] {
			seen[endpoint] = true
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints, labels, nil
}

func clusterRPCURLsForLog(endpoints []string, labels map[string]string) []string {
	result := make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		result[i] = labels[endpoint]
		if result[i] == "" {
			result[i] = rpcURLForLog(endpoint)
		}
	}
	return result
}

func resolveRPCURL(source string) (endpoint, label string, err error) {
	parsed, err := url.Parse(source)
	if err != nil {
		// url.Parse errors can embed the entire input, including credentials.
		return "", "", fmt.Errorf("invalid URL or file URI")
	}
	endpoint = source
	if parsed.Scheme == "file" {
		if !strings.HasPrefix(source, "file://") ||
			(parsed.Host != "" && parsed.Host != "localhost") ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
			strings.Contains(source, "#") || parsed.Opaque != "" ||
			!filepath.IsAbs(parsed.Path) || strings.ContainsRune(parsed.Path, 0) {
			return "", "", fmt.Errorf("file URI must specify an absolute local path without credentials, query, or fragment")
		}
		endpoint, err = readRPCURLFile(parsed.Path)
		if err != nil {
			return "", "", fmt.Errorf("file %s: %w", source, err)
		}
		label = source
	}
	parsed, err = url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		if label != "" {
			return "", "", fmt.Errorf("file %s: first non-blank, non-comment line must be an HTTP/HTTPS URL with a hostname", label)
		}
		return "", "", fmt.Errorf("endpoint must be an HTTP/HTTPS URL with a hostname")
	}
	return endpoint, label, nil
}

func readRPCURLFile(path string) (string, error) {
	// Check before opening so a FIFO or device does not block startup.
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot stat file")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file for reading")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("cannot read a regular file")
	}

	const maxLineBytes = 64 * 1024
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxLineBytes+2)
	for scanner.Scan() {
		if len(scanner.Bytes()) > maxLineBytes {
			return "", fmt.Errorf("line exceeds 64 KiB")
		}
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, nil
		}
	}
	if scanner.Err() != nil {
		return "", fmt.Errorf("cannot read file or line exceeds 64 KiB")
	}
	return "", fmt.Errorf("no non-blank, non-comment URL line")
}
