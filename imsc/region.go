package imsc

import "strings"

// processRegions extracts styled regions that carry a background image.
// Regions without an image are still recorded so paragraphs pointing at them
// can report region_without_background_image instead of silently failing.
func processRegions(root *node) ([]*Region, ErrorList) {
	var regions []*Region
	var errs ErrorList
	seen := map[string]bool{}

	for _, n := range root.collect("region") {
		id, ok := n.attr("id")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			errs.append(NewElementError(n.path, n.Line, ReasonMissingAttr,
				"tt:region requires a non-empty xml:id"))
			continue
		}
		if seen[id] {
			errs.append(NewElementError(n.path, n.Line, ReasonDuplicateID,
				"duplicate tt:region xml:id "+id))
			continue
		}
		seen[id] = true

		region := &Region{ID: id, Line: n.Line}
		if raw, ok := n.attr("backgroundImage"); ok && strings.TrimSpace(raw) != "" {
			for _, ref := range parseBackgroundRefs(raw) {
				region.ImageRefs = append(region.ImageRefs, ref)
				if _, valid := fragmentID(ref); !valid {
					errs.append(NewElementError(n.path, n.Line, ReasonExternalImage,
						"region "+id+" references external or malformed image "+ref+
							"; only same-document fragments are supported"))
				}
			}
			if len(region.ImageRefs) == 0 {
				errs.append(NewElementError(n.path, n.Line, ReasonEmptyAttr,
					"region "+id+" has an empty smpte:backgroundImage"))
			}
		}
		regions = append(regions, region)
	}

	return regions, errs
}
