package imsc

import "strings"

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// parseBackgroundRefs splits an smpte:backgroundImage value into ordered
// same-document fragment references. IMSC allows a space-separated list where
// the first resolvable resource wins.
//
// Each entry must be a same-document fragment ("#id"). External URIs and
// blank entries are rejected by the caller via the returned ref values.
func parseBackgroundRefs(raw string) []string {
	fields := strings.Fields(raw)
	refs := make([]string, 0, len(fields))
	for _, field := range fields {
		refs = append(refs, field)
	}
	return refs
}

// fragmentID strips a leading '#' from a same-document reference. The second
// result is false when the reference is external or malformed.
func fragmentID(ref string) (string, bool) {
	if ref == "" || ref[0] != '#' {
		return "", false
	}
	id := ref[1:]
	if id == "" || strings.ContainsAny(id, " \t\r\n") {
		return "", false
	}
	return id, true
}
