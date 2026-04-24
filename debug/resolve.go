// SPDX-License-Identifier: Apache-2.0

package debug

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ResolveActionID finds the object_id for an action within a microflow.
// The query can be:
//   - "#N" — 1-based index of the action (excluding start/end/merge events)
//   - A string — matched against action caption, result variable name, or action type
func ResolveActionID(mf *microflows.Microflow, query string) (model.ID, error) {
	if mf.ObjectCollection == nil {
		return "", fmt.Errorf("microflow has no actions")
	}

	type actionInfo struct {
		id      model.ID
		caption string
		varName string
		actType string
	}

	var actions []actionInfo
	for _, obj := range mf.ObjectCollection.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		info := actionInfo{
			id:      act.GetID(),
			caption: act.Caption,
		}
		if act.Action != nil {
			info.actType = actionTypeName(act.Action)
			info.varName = actionResultVariable(act.Action)
		}
		actions = append(actions, info)
	}

	if len(actions) == 0 {
		return "", fmt.Errorf("microflow has no action activities")
	}

	if strings.HasPrefix(query, "#") {
		idx, err := strconv.Atoi(query[1:])
		if err != nil || idx < 1 || idx > len(actions) {
			return "", fmt.Errorf("invalid action index %q (valid range: #1 to #%d)", query, len(actions))
		}
		return actions[idx-1].id, nil
	}

	lower := strings.ToLower(query)
	for _, a := range actions {
		if strings.EqualFold(a.caption, query) ||
			strings.EqualFold(a.varName, query) ||
			strings.EqualFold(a.actType, query) {
			return a.id, nil
		}
	}
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.caption), lower) ||
			strings.Contains(strings.ToLower(a.varName), lower) ||
			strings.Contains(strings.ToLower(a.actType), lower) {
			return a.id, nil
		}
	}

	return "", fmt.Errorf("no action matching %q found in microflow (%d actions available)", query, len(actions))
}

func actionTypeName(action microflows.MicroflowAction) string {
	switch action.(type) {
	case *microflows.CreateObjectAction:
		return "CreateObject"
	case *microflows.ChangeObjectAction:
		return "ChangeObject"
	case *microflows.DeleteObjectAction:
		return "DeleteObject"
	case *microflows.CommitObjectsAction:
		return "Commit"
	case *microflows.RollbackObjectAction:
		return "Rollback"
	case *microflows.RetrieveAction:
		return "Retrieve"
	case *microflows.MicroflowCallAction:
		return "MicroflowCall"
	case *microflows.ShowPageAction:
		return "ShowPage"
	case *microflows.ClosePageAction:
		return "ClosePage"
	case *microflows.CreateVariableAction:
		return "CreateVariable"
	case *microflows.ChangeVariableAction:
		return "ChangeVariable"
	case *microflows.LogMessageAction:
		return "LogMessage"
	case *microflows.ValidationFeedbackAction:
		return "ValidationFeedback"
	case *microflows.AggregateListAction:
		return "AggregateList"
	case *microflows.ListOperationAction:
		return "ListOperation"
	case *microflows.CreateListAction:
		return "CreateList"
	case *microflows.ChangeListAction:
		return "ChangeList"
	default:
		return fmt.Sprintf("%T", action)
	}
}

func actionResultVariable(action microflows.MicroflowAction) string {
	switch a := action.(type) {
	case *microflows.CreateObjectAction:
		return a.OutputVariable
	case *microflows.RetrieveAction:
		return a.OutputVariable
	case *microflows.MicroflowCallAction:
		return a.ResultVariableName
	case *microflows.CreateVariableAction:
		return a.VariableName
	case *microflows.ChangeVariableAction:
		return a.VariableName
	case *microflows.AggregateListAction:
		return a.OutputVariable
	case *microflows.CreateListAction:
		return a.OutputVariable
	default:
		return ""
	}
}
