package channelmap

import (
	"reflect"
	"testing"
)

func TestMaskRolesUseBitOrderNotNumericAccident(t *testing.T) {
	roles := rolesOnly(MaskRoles(Mask5Point1))
	want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("0x3F roles = %v, want %v", roles, want)
	}

	side := rolesOnly(MaskRoles(Mask5Point1Side))
	wantSide := []string{"FL", "FR", "FC", "LFE", "SL", "SR"}
	if !reflect.DeepEqual(side, wantSide) {
		t.Fatalf("0x60F roles = %v, want %v", side, wantSide)
	}
}

func TestMaskPopCount(t *testing.T) {
	if got := MaskPopCount(Mask5Point1); got != 6 {
		t.Fatalf("MaskPopCount(0x3F) = %d, want 6", got)
	}
	if got := MaskPopCount(Mask5Point1Side); got != 6 {
		t.Fatalf("MaskPopCount(0x60F) = %d, want 6", got)
	}
}

func rolesOnly(in []Speaker) []string {
	out := make([]string, len(in))
	for i, sp := range in {
		out[i] = sp.Role
	}
	return out
}
