package failover

import (
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
)

// Message represents the message data that can be encoded/decoded
type Message struct {
	CanProceed                       bool
	ErrorMessage                     string
	ActiveNodeInfo                   NodeInfo
	PassiveNodeInfo                  NodeInfo
	IsDryRunFailover                 bool
	IsSuccessfullyCompleted          bool
	SkipTowerSync                    bool
	ActiveNodeSetIdentityStartTime   time.Time
	ActiveNodeSetIdentityEndTime     time.Time
	ActiveNodeSyncTowerFileStartTime time.Time
	ActiveNodeSyncTowerFileEndTime   time.Time
	PassiveNodeSetIdentityStartTime  time.Time
	PassiveNodeSetIdentityEndTime    time.Time
	PassiveNodeSyncTowerFileEndTime  time.Time
	FailoverStartSlot                uint64
	FailoverEndSlot                  uint64
	// key is the identity pubkey
	CreditSamples CreditSamples
}

func (m *Message) currentStateTableString() string {
	activeNodeInfo := m.ActiveNodeInfo
	passiveNodeInfo := m.PassiveNodeInfo
	if m.IsSuccessfullyCompleted && !m.IsDryRunFailover {
		activeNodeInfo = m.PassiveNodeInfo
		passiveNodeInfo = m.ActiveNodeInfo
	}

	rows := [][]string{
		{
			"active",
			activeNodeInfo.Hostname,
			activeNodeInfo.PublicIP,
			activeNodeInfo.Identities.Active.PubKey(),
			activeNodeInfo.ClientVersion,
		},
		{
			"passive",
			passiveNodeInfo.Hostname,
			passiveNodeInfo.PublicIP,
			passiveNodeInfo.Identities.Passive.PubKey(),
			passiveNodeInfo.ClientVersion,
		},
	}
	if m.IsSuccessfullyCompleted && !m.IsDryRunFailover {
		// Reverse the rows
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	return style.RenderTable(
		[]string{"CurrentRole", "AdvertisedName", "PublicIP", "Pubkey", "ClientVersion"},
		rows,
		func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return style.TableHeaderStyle
			}

			// first data row is always active peer so style it active color
			rowStyle := style.TableCellStyle
			roleString := rows[row][0]
			if roleString == "active" {
				rowStyle = rowStyle.Foreground(style.ColorActive)
			}
			if roleString == "passive" {
				rowStyle = rowStyle.Foreground(style.ColorPassive)
			}

			// resize columns a little bit for table
			switch col {
			case 0: // role active  (them)
				return rowStyle.Width(14)
			case 1: // advertised name
				return rowStyle.Width(45)
			case 2: // IP
				return rowStyle.Width(18).Align(lipgloss.Left)
			case 3: // pubkey
				return rowStyle.Width(46)
			case 4: // ClientVersion
				return rowStyle.Width(18)
			}
			return rowStyle
		},
	)
}
