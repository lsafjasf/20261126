package channelmap

import "fmt"

// SignaturesFromReference trusts the reference WAV mask order and copies an
// aligned sample template for every declared role. It enables matching arbitrary
// characteristic test material without inventing a frequency rule.
func SignaturesFromReference(ref *File) (*SignatureSet, error) {
	if ref == nil {
		return nil, fmt.Errorf("reference: nil WAV file")
	}
	if len(ref.Samples) == 0 {
		return nil, fmt.Errorf("reference: no decoded PCM/float samples")
	}

	var roles []Speaker
	switch {
	case ref.Format.HasChannelMask && ref.Format.ChannelMask != 0:
		roles = MaskRoles(ref.Format.ChannelMask)
	default:
		if mask, ok := DefaultMaskForChannels(ref.Format.Channels); ok {
			roles = MaskRoles(mask)
		}
	}

	set := &SignatureSet{
		Name:       "reference-template-v1",
		SampleRate: ref.Format.SampleRate,
		Roles:      make(map[string]RoleSignature, ref.Format.Channels),
	}
	const maxTemplate = 16384
	for wire := 0; wire < ref.Format.Channels && wire < len(ref.Samples); wire++ {
		role := fmt.Sprintf("CHANNEL_%d", wire)
		if wire < len(roles) {
			role = roles[wire].Role
		}
		src := ref.Samples[wire]
		n := len(src)
		if n > maxTemplate {
			n = maxTemplate
		}
		template := append([]float64(nil), src[:n]...)
		set.Roles[role] = RoleSignature{Template: template, TemplateStart: 0}
	}
	set.Description = "Templates copied from a caller-declared known-good reference WAV; reference mask order is trusted."
	return set, nil
}
