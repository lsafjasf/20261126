// Package cea708 implements CEA-708 service demultiplexing, per-service
// window/screen state, safe service switching and resumable recovery.
//
// The reported defect is "old windows briefly overlap the new service"
// (叠字) when a program switches between service 1 and service 2. The root
// cause is shared retained geometry: a single window array/framebuffer is
// reused across services and the new service is composited over the previous
// frame before an explicit clear boundary.
//
// The fix has four layers:
//
//   - Service selection: Manager.SelectService atomically changes the visible
//     service and emits a FrameSwitch composited on a freshly cleared canvas.
//   - Window latching: every service owns eight Window values plus a latched
//     Frame. Switching away freezes that state; switching back restores it
//     exactly. Presented frames are immutable value snapshots.
//   - Clear rules: CLW erases text but keeps the window, HDW hides while
//     retaining text, DSW/TGW change visibility, DLW deletes the window and
//     RST resets the whole service. A service switch additionally records a
//     ClearServiceSwitch event and clears the compositor before recomposition.
//   - Independent screens and recovery: Manager.Snapshot serializes every
//     service screen into a RecoveryPoint token. After a network outage the
//     client rebuilds either from that snapshot plus suffix replay or from the
//     nearest clean RST/reset boundary; sequence deduplication makes replay
//     exactly-once in effect.
//
// The package is pure Go with no external dependencies. It includes a small
// CEA-708 command interpreter, DTVCC packet/service-block framer and cc_data
// reassembler so the switching and recovery flows can be driven from wire
// bytes as well as from the structured API.
package cea708
