package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var (
	rpcClient       *wshutil.WshRpc
	rpcClientOnce   sync.Once
	rpcClientErr    error
	_               string // agent JWT used inline, not stored
	agentJwtExpiry  time.Time
	agentJwtMu      sync.Mutex
)

const (
	agentTokenDuration       = 60 * time.Minute
	agentTokenRefreshMargin  = 5 * time.Minute
)

func getRpcClient() (*wshutil.WshRpc, error) {
	rpcClientOnce.Do(func() {
		jwt := os.Getenv("WAVETERM_JWT")
		if jwt == "" {
			rpcClientErr = fmt.Errorf("WAVETERM_JWT not set; wave-mcp must run inside a Wave session")
			return
		}
		sockName, err := wshutil.ExtractUnverifiedSocketName(jwt)
		if err != nil {
			rpcClientErr = fmt.Errorf("extracting socket name from JWT: %w", err)
			return
		}
		client, err := wshutil.SetupDomainSocketRpcClient(sockName, nil, "wave-mcp")
		if err != nil {
			rpcClientErr = fmt.Errorf("connecting to domain socket %s: %w", sockName, err)
			return
		}
		_, err = wshclient.AuthenticateCommand(client, jwt, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
		if err != nil {
			rpcClientErr = fmt.Errorf("authenticating with JWT: %w", err)
			return
		}

		agentRtn, err := wshclient.AgentIssueTokenCommand(client, wshrpc.AgentIssueTokenData{
			DurationMinutes: int(agentTokenDuration.Minutes()),
		}, &wshrpc.RpcOpts{Timeout: 5000})
		if err != nil {
			rpcClientErr = fmt.Errorf("issuing agent JWT: %w", err)
			return
		}

		_, err = wshclient.AuthenticateCommand(client, agentRtn.Token, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
		if err != nil {
			rpcClientErr = fmt.Errorf("authenticating with agent JWT: %w", err)
			return
		}

		agentJwtExpiry = time.Now().Add(agentTokenDuration - agentTokenRefreshMargin)
		rpcClient = client
	})
	return rpcClient, rpcClientErr
}

func ensureAgentClient() (*wshutil.WshRpc, error) {
	client, err := getRpcClient()
	if err != nil {
		return nil, err
	}
	agentJwtMu.Lock()
	defer agentJwtMu.Unlock()
	if time.Now().Before(agentJwtExpiry) {
		return client, nil
	}
	rtn, err := wshclient.AgentIssueTokenCommand(client, wshrpc.AgentIssueTokenData{
		DurationMinutes: int(agentTokenDuration.Minutes()),
	}, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return nil, fmt.Errorf("refreshing agent JWT: %w", err)
	}
	_, err = wshclient.AuthenticateCommand(client, rtn.Token, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
	if err != nil {
		return nil, fmt.Errorf("re-authenticating with refreshed agent JWT: %w", err)
	}
	agentJwtExpiry = time.Now().Add(agentTokenDuration - agentTokenRefreshMargin)
	return client, nil
}

type WidgetInfo struct {
	BlockId   string `json:"block_id"`
	TabId     string `json:"tab_id"`
	ViewType  string `json:"view_type"`
	ShortDesc string `json:"short_desc,omitempty"`
}

func rpcListWidgets() ([]WidgetInfo, error) {
	client, err := ensureAgentClient()
	if err != nil {
		return nil, err
	}

	entries, err := wshclient.AgentListBlocksCommand(client, wshrpc.AgentListBlocksData{
		BlockType: "term",
	}, &wshrpc.RpcOpts{Timeout: 10000})
	if err != nil {
		return nil, fmt.Errorf("listing blocks: %w", err)
	}

	var widgets []WidgetInfo
	for _, b := range entries {
		widgets = append(widgets, WidgetInfo{
			BlockId:  b.BlockId,
			TabId:    b.TabId,
			ViewType: "term",
		})
	}
	return widgets, nil
}

func rpcGetScrollback(blockId string, lineStart, lineEnd int) (*wshrpc.CommandTermGetScrollbackLinesRtnData, error) {
	client, err := ensureAgentClient()
	if err != nil {
		return nil, err
	}
	data := wshrpc.AgentGetScrollbackData{
		BlockId:   blockId,
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}
	opts := &wshrpc.RpcOpts{Timeout: int64(termScrollbackTimeoutMs)}
	return wshclient.AgentGetScrollbackCommand(client, data, opts)
}

func rpcTermInfo(blockId string) (*wshrpc.AgentListTerminalsRtnData, error) {
	client, err := ensureAgentClient()
	if err != nil {
		return nil, err
	}
	return wshclient.AgentListTerminalsCommand(client, wshrpc.AgentListTerminalsData{}, &wshrpc.RpcOpts{Timeout: 5000})
}

func rpcSendInput(blockId string, text string, enter bool) error {
	client, err := ensureAgentClient()
	if err != nil {
		return err
	}
	data := wshrpc.AgentSendInputData{
		BlockId:   blockId,
		InputData: text,
		Enter:     enter,
	}
	opts := &wshrpc.RpcOpts{Timeout: 10000}
	return wshclient.AgentSendInputCommand(client, data, opts)
}

func rpcRunCommand(command string, timeoutMs int) (*wshrpc.AgentRunCommandRtnData, error) {
	client, err := ensureAgentClient()
	if err != nil {
		return nil, err
	}
	data := wshrpc.AgentRunCommandData{
		Command: command,
		Timeout: timeoutMs / 1000,
	}
	opts := &wshrpc.RpcOpts{Timeout: int64(timeoutMs) + 2000}
	return wshclient.AgentRunCommandCommand(client, data, opts)
}

func rpcTermSearchScrollback(blockId, pattern string, isRegex bool, maxMatches int) (*wshrpc.CommandTermSearchScrollbackRtnData, error) {
	client, err := ensureAgentClient()
	if err != nil {
		return nil, err
	}
	data := wshrpc.CommandTermSearchScrollbackData{
		BlockId:    blockId,
		Pattern:    pattern,
		IsRegex:    isRegex,
		MaxMatches: maxMatches,
	}
	opts := &wshrpc.RpcOpts{
		Timeout: 10000,
		Route:   wshutil.MakeFeBlockRouteId(blockId),
	}
	return wshclient.TermSearchScrollbackCommand(client, data, opts)
}

func rpcWidgetClearScrollback(blockId string) error {
	client, err := ensureAgentClient()
	if err != nil {
		return err
	}
	data := wshrpc.WidgetClearScrollbackData{BlockId: blockId}
	opts := &wshrpc.RpcOpts{
		Timeout: 10000,
		Route:   wshutil.MakeFeBlockRouteId(blockId),
	}
	return wshclient.WidgetClearScrollbackCommand(client, data, opts)
}
