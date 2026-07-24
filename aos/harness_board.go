package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"
)

const (
	harnessBoardFormat    = "agentic-os.role-harness-board.v1"
	harnessBoardRoleCount = 10
	harnessBoardLaneCount = 16
)

//go:embed role-harnesses.json
var embeddedHarnessBoard []byte

type harnessLane struct {
	Intent  string `json:"intent"`
	Harness string `json:"harness"`
}

type harnessRole struct {
	Role    string        `json:"role"`
	Intents []harnessLane `json:"intents"`
}

type harnessBoard struct {
	Format     string        `json:"format"`
	RoleSource string        `json:"role_source"`
	RoleCount  int           `json:"role_count"`
	LaneCount  int           `json:"lane_count"`
	Roles      []harnessRole `json:"roles"`
}

func loadHarnessBoard(data []byte) (harnessBoard, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var board harnessBoard
	if err := decoder.Decode(&board); err != nil {
		return harnessBoard{}, fmt.Errorf("decode embedded harness board: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return harnessBoard{}, err
	}
	if board.Format != harnessBoardFormat {
		return harnessBoard{}, fmt.Errorf("unsupported harness board format %q", board.Format)
	}
	if strings.TrimSpace(board.RoleSource) == "" {
		return harnessBoard{}, errors.New("harness board role_source is empty")
	}
	if board.RoleCount != len(board.Roles) {
		return harnessBoard{}, fmt.Errorf(
			"harness board role_count is %d, found %d roles",
			board.RoleCount,
			len(board.Roles),
		)
	}
	if board.RoleCount != harnessBoardRoleCount {
		return harnessBoard{}, fmt.Errorf(
			"harness board has %d roles, want %d",
			board.RoleCount,
			harnessBoardRoleCount,
		)
	}

	roles := make(map[string]struct{}, len(board.Roles))
	laneCount := 0
	for _, role := range board.Roles {
		if strings.TrimSpace(role.Role) == "" {
			return harnessBoard{}, errors.New("harness board contains an empty role")
		}
		if _, exists := roles[role.Role]; exists {
			return harnessBoard{}, fmt.Errorf("harness board repeats role %q", role.Role)
		}
		roles[role.Role] = struct{}{}
		if len(role.Intents) < 1 || len(role.Intents) > 2 {
			return harnessBoard{}, fmt.Errorf(
				"harness board role %q has %d intents, want one or two",
				role.Role,
				len(role.Intents),
			)
		}
		intents := make(map[string]struct{}, len(role.Intents))
		for _, lane := range role.Intents {
			if strings.TrimSpace(lane.Intent) == "" || strings.TrimSpace(lane.Harness) == "" {
				return harnessBoard{}, fmt.Errorf(
					"harness board %s contains an empty intent or harness",
					role.Role,
				)
			}
			if _, exists := intents[lane.Intent]; exists {
				return harnessBoard{}, fmt.Errorf(
					"harness board repeats lane %s/%s",
					role.Role,
					lane.Intent,
				)
			}
			intents[lane.Intent] = struct{}{}
			laneCount++
		}
	}
	if board.LaneCount != laneCount {
		return harnessBoard{}, fmt.Errorf(
			"harness board lane_count is %d, found %d lanes",
			board.LaneCount,
			laneCount,
		)
	}
	if board.LaneCount != harnessBoardLaneCount {
		return harnessBoard{}, fmt.Errorf(
			"harness board has %d lanes, want %d",
			board.LaneCount,
			harnessBoardLaneCount,
		)
	}
	return board, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode embedded harness board trailer: %w", err)
	}
	return errors.New("embedded harness board contains multiple JSON values")
}

func resolveHarnessDefault(board harnessBoard, role, intent string) (string, error) {
	for _, candidate := range board.Roles {
		if candidate.Role != role {
			continue
		}
		for _, lane := range candidate.Intents {
			if lane.Intent == intent {
				return lane.Harness, nil
			}
		}
		return "", fmt.Errorf("role %q has no intent %q", role, intent)
	}
	return "", fmt.Errorf("unknown role %q", role)
}

func runHarnessDefault(_ context.Context, cmd *cli.Command) error {
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return errors.New("harness-default needs --role")
	}
	intent := strings.TrimSpace(cmd.String("intent"))
	if intent == "" {
		return errors.New("harness-default needs --intent")
	}
	board, err := loadHarnessBoard(embeddedHarnessBoard)
	if err != nil {
		return err
	}
	harness, err := resolveHarnessDefault(board, role, intent)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.Root().Writer, harness)
	return nil
}
