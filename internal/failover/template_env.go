package failover

import "fmt"

// AddHandoffTemplateEnv adds directional metadata to hook environments.
// Vote-history keys replace the former Tower-specific metadata.
func AddHandoffTemplateEnv(env map[string]string, source, destination NodeInfo, local, peer NodeInfo, strategy string, voteHistoryWillBeTransferred bool) {
	env["THIS_NODE_CLIENT_FAMILY"] = local.ClientFamily
	env["PEER_NODE_CLIENT_FAMILY"] = peer.ClientFamily
	env["THIS_NODE_IS_NATIVE_FIREDANCER"] = fmt.Sprintf("%t", local.IsNativeFiredancer)
	env["PEER_NODE_IS_NATIVE_FIREDANCER"] = fmt.Sprintf("%t", peer.IsNativeFiredancer)
	env["FROM_NODE_CLIENT_FAMILY"] = source.ClientFamily
	env["TO_NODE_CLIENT_FAMILY"] = destination.ClientFamily
	env["FROM_NODE_IS_NATIVE_FIREDANCER"] = fmt.Sprintf("%t", source.IsNativeFiredancer)
	env["TO_NODE_IS_NATIVE_FIREDANCER"] = fmt.Sprintf("%t", destination.IsNativeFiredancer)
	env["FROM_NODE_IS_AGAVE_DERIVED"] = fmt.Sprintf("%t", isAgaveDerived(source.ClientFamily))
	env["TO_NODE_IS_AGAVE_DERIVED"] = fmt.Sprintf("%t", isAgaveDerived(destination.ClientFamily))
	env["HANDOFF_STRATEGY"] = strategy
	env["VOTE_HISTORY_WILL_BE_TRANSFERRED"] = fmt.Sprintf("%t", voteHistoryWillBeTransferred)
	env["VOTE_HISTORY_FILE"] = local.VoteHistoryFile
	env["VOTE_HISTORY_IMPORT_FILE"] = local.VoteHistoryImportFile
}
