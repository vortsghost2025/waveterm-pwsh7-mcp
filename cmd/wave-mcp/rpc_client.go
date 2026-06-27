package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"sync"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var (
	rpcClient     *wshutil.WshRpc
	rpcClientOnce sync.Once
	rpcClientErr  error
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
		rpcClient = client
	})
	return rpcClient, rpcClientErr
}

type WidgetInfo struct {
	BlockId   string `json:"block_id"`
	TabId     string `json:"tab_id"`
	ViewType  string `json:"view_type"`
	ShortDesc string `json:"short_desc,omitempty"`
}

func rpcListWidgets() ([]WidgetInfo, error) {
	client, err := getRpcClient()
	if err != nil {
		return nil, err
	}

	workspaces, err := wshclient.WorkspaceListCommand(client, &wshrpc.RpcOpts{Timeout: 10000})
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}

	var widgets []WidgetInfo
	for _, ws := range workspaces {
		blocks, err := wshclient.BlocksListCommand(client, wshrpc.BlocksListRequest{
			WorkspaceId: ws.WorkspaceData.OID,
		}, &wshrpc.RpcOpts{Timeout: 10000})
		if err != nil {
			continue
		}
		for _, b := range blocks {
			viewType := b.Meta.GetString("view", "")
			if viewType != "term" {
				continue
			}
			widgets = append(widgets, WidgetInfo{
				BlockId:   b.BlockId,
				TabId:     b.TabId,
				ViewType:  viewType,
			})
		}
	}
	return widgets, nil
}

func rpcGetScrollback(blockId string, lineStart, lineEnd int) (*wshrpc.CommandTermGetScrollbackLinesRtnData, error) {
	client, err := getRpcClient()
	if err != nil {
		return nil, err
	}
	data := wshrpc.CommandTermGetScrollbackLinesData{
		LineStart: lineStart,
		LineEnd:   lineEnd,
	}
	opts := &wshrpc.RpcOpts{
		Route:   wshutil.MakeFeBlockRouteId(blockId),
		Timeout: int64(termScrollbackTimeoutMs),
	}
	return wshclient.TermGetScrollbackLinesCommand(client, data, opts)
}

func rpcTermInfo(blockId string) (*wshrpc.TermInfo, error) {
	client, err := getRpcClient()
	if err != nil {
		return nil, err
	}
	return wshclient.TermInfoCommand(client, wshrpc.TermInfoRequest{BlockID: blockId}, &wshrpc.RpcOpts{Timeout: 5000})
}

func rpcSendInput(blockId string, text string) error {
	client, err := getRpcClient()
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	data := wshrpc.CommandBlockInputData{
		BlockId:     blockId,
		InputData64: encoded,
	}
	opts := &wshrpc.RpcOpts{
		Route:   wshutil.MakeFeBlockRouteId(blockId),
		Timeout: 10000,
	}
	return wshclient.ControllerInputCommand(client, data, opts)
}
