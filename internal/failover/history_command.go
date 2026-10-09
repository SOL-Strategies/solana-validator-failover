package failover

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

const NativeHistoryMaxSize int64 = 32688
const MinimumFiredancerVersion = "26.10.0"

func ValidateFiredancerVersion(version string) error {
	fields := strings.Fields(version)
	if len(fields) == 0 {
		return fmt.Errorf("native Firedancer version is unavailable; require v%s or newer", MinimumFiredancerVersion)
	}
	value := strings.TrimPrefix(fields[0], "v")
	base, _, _ := strings.Cut(value, "+")
	base, _, prerelease := strings.Cut(base, "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return fmt.Errorf("unrecognized native Firedancer version %q; require v%s or newer", version, MinimumFiredancerVersion)
	}
	var numbers [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return fmt.Errorf("unrecognized native Firedancer version %q", version)
		}
		numbers[i] = n
	}
	newer := numbers[0] > 26 || numbers[0] == 26 && (numbers[1] > 10 || numbers[1] == 10 && numbers[2] > 0)
	equal := numbers == [3]int{26, 10, 0}
	if !newer && !(equal && !prerelease) {
		return fmt.Errorf("native Firedancer %q is unsupported; require v%s or newer", version, MinimumFiredancerVersion)
	}
	return nil
}

func ValidateNativeVersion(info *NodeInfo) error {
	if !info.IsNativeFiredancer {
		return nil
	}
	if info.ClientFamily != "firedancer" {
		return fmt.Errorf("inconsistent native Firedancer client family %q", info.ClientFamily)
	}
	return ValidateFiredancerVersion(info.ClientVersionRPC)
}

// ValidateHistoryCommand checks exactly the argument vector execution will use.
func ValidateHistoryCommand(command, expected string) error {
	args, err := utils.CommandArgs(command)
	if err != nil {
		return err
	}
	count := 0
	actual := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--vote-history-file" {
			count++
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return fmt.Errorf("--vote-history-file requires a path; expected %s", expected)
			}
			i++
			actual = args[i]
		} else if strings.HasPrefix(arg, "--vote-history-file=") {
			count++
			actual = strings.TrimPrefix(arg, "--vote-history-file=")
		}
	}
	if count != 1 || actual == "" {
		return fmt.Errorf("Firedancer activation must supply exactly one --vote-history-file with path %s (found %d)", expected, count)
	}
	commandPath, err := filepath.Abs(actual)
	if err != nil {
		return err
	}
	transferPath, err := filepath.Abs(expected)
	if err != nil {
		return err
	}
	if commandPath != transferPath {
		return fmt.Errorf("Firedancer --vote-history-file path differs from transfer destination: command=%s transfer=%s", commandPath, transferPath)
	}
	return nil
}
