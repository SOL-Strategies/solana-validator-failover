package failover

import (
	"bytes"
	"fmt"
	"reflect"
	"text/template"

	"github.com/sol-strategies/solana-validator-failover/internal/identities"
)

// CommandTemplateData is the public template context for operator-controlled
// identity commands. The legacy fields intentionally mirror Validator so
// existing templates continue to work.
type CommandTemplateData struct {
	Bin                   string
	LedgerDir             string
	ClientConfigPath      string
	Identities            *identities.Identities
	VoteHistoryFile       string
	VoteHistoryImportFile string
	Name                  string
	// Hostname is retained as an alias for Name for backwards compatibility.
	Hostname   string
	PublicIP   string
	RPCAddress string

	ThisNodeClientFamily         string
	PeerNodeClientFamily         string
	FromNodeClientFamily         string
	ToNodeClientFamily           string
	ThisNodeIsNativeFiredancer   bool
	PeerNodeIsNativeFiredancer   bool
	FromNodeIsNativeFiredancer   bool
	ToNodeIsNativeFiredancer     bool
	FromNodeIsAgaveDerived       bool
	ToNodeIsAgaveDerived         bool
	FromNodeConsensus            string
	ToNodeConsensus              string
	FromNodeClientVersion        string
	ToNodeClientVersion          string
	HandoffStrategy              string
	VoteHistoryWillBeTransferred bool
	ActiveIdentityPubkey         string
	VoteAccountPubkey            string
	IsDryRunFailover             bool
}

func isAgaveDerived(family string) bool {
	return family == "agave" || family == "jito-solana"
}

// NewCommandTemplateData builds a directional context. local is the node on
// which the command will run; source and destination are the failover roles.
func NewCommandTemplateData(local, peer, source, destination NodeInfo, strategy string, voteHistoryWillBeTransferred, dryRun bool) CommandTemplateData {
	return CommandTemplateData{
		Bin: local.Bin, LedgerDir: local.LedgerDir, ClientConfigPath: local.ClientConfigPath, Identities: local.Identities,
		VoteHistoryFile: local.VoteHistoryFile, VoteHistoryImportFile: local.VoteHistoryImportFile, Name: local.Hostname, Hostname: local.Hostname, PublicIP: local.PublicIP,
		RPCAddress:           local.RPCAddress,
		ThisNodeClientFamily: local.ClientFamily, PeerNodeClientFamily: peer.ClientFamily,
		FromNodeClientFamily: source.ClientFamily, ToNodeClientFamily: destination.ClientFamily,
		ThisNodeIsNativeFiredancer: local.IsNativeFiredancer,
		PeerNodeIsNativeFiredancer: peer.IsNativeFiredancer,
		FromNodeIsNativeFiredancer: source.IsNativeFiredancer,
		ToNodeIsNativeFiredancer:   destination.IsNativeFiredancer,
		FromNodeIsAgaveDerived:     isAgaveDerived(source.ClientFamily),
		ToNodeIsAgaveDerived:       isAgaveDerived(destination.ClientFamily),
		FromNodeConsensus:          source.ConsensusMode, ToNodeConsensus: destination.ConsensusMode,
		FromNodeClientVersion: source.ClientVersion, ToNodeClientVersion: destination.ClientVersion,
		HandoffStrategy:              strategy,
		VoteHistoryWillBeTransferred: voteHistoryWillBeTransferred,
		ActiveIdentityPubkey:         source.Identities.Active.PubKey(),
		VoteAccountPubkey:            source.VoteAccount,
		IsDryRunFailover:             dryRun,
	}
}

// RenderIdentityCommand renders an operator-supplied command after peer
// negotiation has established the directional failover context.
func RenderIdentityCommand(commandTemplate string, data CommandTemplateData) (string, error) {
	if commandTemplate == "" {
		return "", nil
	}
	tpl, err := template.New("identity-command").Funcs(template.FuncMap{
		// neq is a convenience alias for the standard Go-template `ne`.
		"neq": func(a, b any) bool { return !reflect.DeepEqual(a, b) },
	}).Parse(commandTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse identity command template: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render identity command template: %w", err)
	}
	return buf.String(), nil
}
