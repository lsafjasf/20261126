package eac3

// PlanSpecChange 是无状态入口：比较前后两个解码前节目规格，返回有序的
// 受控重建事件。需要节目拓扑、审计边和当前状态时使用 Graph.Switch。
func PlanSpecChange(from, to ProgramSpec, changeReason string) []RebuildEvent {
	oldState := &State{ProgramID: "spec", Spec: from}
	newState := &State{ProgramID: "spec", Spec: to}
	req := SwitchRequest{Reason: changeReason}
	reasons := buildReasons(oldState, newState, req)
	boundary := buildBoundary(oldState, newState, req)
	return buildEvents(oldState, newState, req, reasons, boundary)
}

// PlanInitialSpec 返回首个节目规格所需的初始化事件。
func PlanInitialSpec(spec ProgramSpec) []RebuildEvent {
	state := &State{ProgramID: "initial", Spec: spec}
	req := SwitchRequest{}
	reasons := buildReasons(nil, state, req)
	boundary := buildBoundary(nil, state, req)
	return buildEvents(nil, state, req, reasons, boundary)
}
