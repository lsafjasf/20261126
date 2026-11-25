package fcpxml

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseTestXML(t *testing.T, body string) *Document {
	t.Helper()
	doc, err := ParseBytes([]byte(body))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	return doc
}

func TestSameNameIsolatedByEffectTypeAndSource(t *testing.T) {
	input := `<fcpxml version="1.10">
  <resources>
    <effect id="r1" name="Blur" effectID="com.example.blur"/>
    <effect id="r2" name="Glow" effectID="com.example.glow"/>
  </resources>
  <sequence><spine><clip>
    <filter-video ref="r1"><param name="Amount" key="amount" value="0.25"/></filter-video>
    <filter-video ref="r2">
      <param name="Amount" key="amount">
        <param name="Center" key="center" value="0.5"/>
      </param>
    </filter-video>
  </clip></spine></sequence>
</fcpxml>`

	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 3 {
		t.Fatalf("got %d parameters, want 3: %#v", len(params), params)
	}

	first := params[0]
	second := findParam(t, params, "param[Amount]", "com.example.glow")
	if first.Namespace() == second.Namespace() {
		t.Fatalf("same-name parameters collided: %q", first.Namespace())
	}
	if first.EffectType != "com.example.blur" || second.EffectType != "com.example.glow" {
		t.Fatalf("effect types not resolved: %q, %q", first.EffectType, second.EffectType)
	}
	if first.Source != "param[Amount]" || second.Source != "param[Amount]" {
		t.Fatalf("unexpected sources: %q, %q", first.Source, second.Source)
	}
	if first.Value.Text != "0.25" {
		t.Fatalf("scalar value = %q, want 0.25", first.Value.Text)
	}
	if second.Value.Kind != ValueStructured || len(second.Value.Fields) != 1 {
		t.Fatalf("structured value not retained: %#v", second.Value)
	}
}

func TestLegacyAndModernParameterSourcesDoNotCollide(t *testing.T) {
	input := `<fcpxml version="1.10">
  <resources>
    <effect id="r1" name="Color" effectID="com.example.color">
      <parameter>
        <name>Amount</name><parameterid>amount</parameterid>
        <value><horiz>1.0</horiz><vert>2.0</vert></value>
      </parameter>
    </effect>
  </resources>
  <clip>
    <filter-video ref="r1">
      <param name="Amount" key="amount"><param name="Scale" value="3.0"/></param>
    </filter-video>
  </clip>
</fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 3 {
		t.Fatalf("got %d parameters, want 3", len(params))
	}
	legacy := findParam(t, params, "parameter[Amount]", "com.example.color")
	modern := findParam(t, params, "param[Amount]", "com.example.color")
	if legacy.Namespace() == modern.Namespace() {
		t.Fatal("legacy and modern parameter sources collided")
	}
	if legacy.Value.Kind != ValueStructured || len(legacy.Value.Fields) != 2 {
		t.Fatalf("legacy structured value lost: %#v", legacy.Value)
	}
	if legacy.ParameterID != "amount" {
		t.Fatalf("parameter ID = %q", legacy.ParameterID)
	}
}

func TestScalarAndStructuredSameSourceDoNotCollide(t *testing.T) {
	input := `<fcpxml version="1.10">
  <resources><effect id="r1" name="Blur" effectID="blur"/></resources>
  <clip><filter-video ref="r1">
    <param name="Amount" key="amount" value="1"/>
    <param name="Amount" key="amount"><param name="Center" value="0.5"/></param>
  </filter-video></clip>
</fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 3 {
		t.Fatalf("got %d parameters, want 3", len(params))
	}
	scalar := findParamKind(t, params, ValueScalar, "param[Amount]")
	structured := findParamKind(t, params, ValueStructured, "param[Amount]")
	if scalar.Namespace() == structured.Namespace() {
		t.Fatalf("scalar and structured parameters collided: %q", scalar.Namespace())
	}
	if scalar.NamespaceID != scalar.Namespace() || structured.NamespaceID != structured.Namespace() {
		t.Fatal("reported namespace is not the computed namespace")
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("both values are mapped but save was blocked: %v", err)
	}
}

func TestUnsupportedValueKeepsRawAndBlocksSave(t *testing.T) {
	input := `<fcpxml version="1.10">
  <resources><effect id="r1" name="Blur" effectID="blur"/></resources>
  <clip><filter-video ref="r1"><data key="effectData">AQIDBA==</data></filter-video></clip>
</fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 1 || params[0].Mapped {
		t.Fatalf("binary data should be retained but unmapped: %#v", params)
	}
	if params[0].Raw == "" {
		t.Fatal("raw binary parameter was not retained")
	}
	if err := doc.Validate(); !errors.Is(err, ErrLossySave) {
		t.Fatalf("Validate error = %v, want ErrLossySave", err)
	}

	var out bytes.Buffer
	if err := doc.Save(&out); !errors.Is(err, ErrLossySave) {
		t.Fatalf("Save error = %v, want ErrLossySave", err)
	}
	if out.Len() != 0 {
		t.Fatalf("lossy save wrote %d bytes", out.Len())
	}

	path := filepath.Join(t.TempDir(), "existing.fcpxml")
	original := []byte("do not replace")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := doc.SaveFile(path); !errors.Is(err, ErrLossySave) {
		t.Fatalf("SaveFile error = %v, want ErrLossySave", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("lossy SaveFile replaced destination with %q", got)
	}
}

func TestAnimatedLegacyParameterIsNotSilentlyFlattened(t *testing.T) {
	input := `<fcpxml version="1.10"><resources>
  <effect id="r1" name="Color" effectID="color">
    <parameter><name>Amount</name><parameterid>amount</parameterid>
      <keyframe><when>0s</when><value>1</value></keyframe>
    </parameter>
  </effect>
</resources></fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 1 || params[0].Mapped {
		t.Fatalf("animated parameter should be retained as unmapped: %#v", params)
	}
	if !strings.Contains(params[0].Raw, "<keyframe>") {
		t.Fatalf("raw keyframe data was not retained: %q", params[0].Raw)
	}
	if err := doc.Validate(); !errors.Is(err, ErrLossySave) {
		t.Fatalf("animated parameter did not block save: %v", err)
	}
}

func TestNestedDataBelongsToParentParameter(t *testing.T) {
	input := `<fcpxml version="1.10"><resources><effect id="r1" name="Blur"/></resources>
  <clip><filter-video ref="r1">
    <param name="Blob" key="blob"><data key="effectData">AQID</data></param>
  </filter-video></clip>
</fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 1 {
		t.Fatalf("nested data was indexed as an extra parameter: %#v", params)
	}
	if params[0].Name != "Blob" || params[0].Mapped {
		t.Fatalf("parent parameter was not preserved and blocked: %#v", params[0])
	}
	if !strings.Contains(params[0].Raw, "<data") {
		t.Fatal("parent raw value does not retain nested data")
	}
}

func TestUnknownStructuredAttributeIsLossy(t *testing.T) {
	input := `<fcpxml version="1.10"><resources>
  <effect id="r1" name="Color" effectID="color">
    <parameter><name>Amount</name><value><axis mode="xy">1</axis></value></parameter>
  </effect>
</resources></fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 1 || params[0].Mapped {
		t.Fatalf("unknown value attribute should be unmapped: %#v", params)
	}
	if !strings.Contains(params[0].Raw, `mode="xy"`) {
		t.Fatal("raw value attribute was not retained")
	}
	if err := doc.Validate(); !errors.Is(err, ErrLossySave) {
		t.Fatalf("lossy value did not block save: %v", err)
	}
}

func TestKeyframeNestedInLegacyValueIsLossy(t *testing.T) {
	input := `<fcpxml version="1.10"><resources>
  <effect id="r1" name="Color">
    <parameter><name>Amount</name>
      <value><keyframe><when>0s</when><value>1</value></keyframe></value>
    </parameter>
  </effect>
</resources></fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 1 || params[0].Mapped {
		t.Fatalf("nested keyframe should be unmapped: %#v", params)
	}
	if err := doc.Validate(); !errors.Is(err, ErrLossySave) {
		t.Fatalf("nested keyframe did not block save: %v", err)
	}
}

func TestParseRejectsMultipleDocumentElements(t *testing.T) {
	if _, err := ParseBytes([]byte(`<fcpxml/><fcpxml/>`)); err == nil {
		t.Fatal("ParseBytes accepted multiple document elements")
	}
}

func findParam(t *testing.T, params []Parameter, source, effectType string) Parameter {
	t.Helper()
	for _, param := range params {
		if param.Source == source && param.EffectType == effectType {
			return param
		}
	}
	t.Fatalf("parameter source=%q effectType=%q not found in %#v", source, effectType, params)
	return Parameter{}
}

func findParamKind(t *testing.T, params []Parameter, kind ValueKind, source string) Parameter {
	t.Helper()
	for _, param := range params {
		if param.Value.Kind == kind && param.Source == source && param.EffectType == "blur" {
			return param
		}
	}
	t.Fatalf("parameter kind=%q source=%q not found in %#v", kind, source, params)
	return Parameter{}
}

func TestImportIsStableAcrossInputOrder(t *testing.T) {
	resources := []string{
		`<effect id="r1" name="Blur" effectID="com.example.blur"/>`,
		`<effect id="r2" name="Glow" effectID="com.example.glow"/>`,
		`<effect id="r3" name="Tone" effectID="com.example.tone"/>`,
	}
	effects := []string{
		`<filter-video ref="r1"><param name="Amount" key="amount" value="0.25"/></filter-video>`,
		`<filter-video ref="r2"><param name="Amount" key="amount"><param name="Center" value="0.5"/></param></filter-video>`,
		`<filter-audio ref="r3"><param name="Amount" key="amount" value="-3dB"/></filter-audio>`,
	}

	var reference Snapshot
	var referenceJSON []byte
	for _, order := range [][3]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	} {
		body := `<fcpxml version="1.10"><resources>` +
			resources[order[0]] + resources[order[1]] + resources[order[2]] +
			`</resources><clip>` +
			effects[order[0]] + effects[order[1]] + effects[order[2]] +
			`</clip></fcpxml>`
		doc := parseTestXML(t, body)
		got := doc.Snapshot()
		gotJSON, err := doc.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if referenceJSON == nil {
			reference, referenceJSON = got, gotJSON
			continue
		}
		if !reflect.DeepEqual(got, reference) {
			t.Fatalf("snapshot changed with order %v:\nfirst=%#v\nnow=%#v", order, reference, got)
		}
		if !bytes.Equal(gotJSON, referenceJSON) {
			t.Fatalf("canonical JSON changed with order %v\nfirst:\n%s\nnow:\n%s", order, referenceJSON, gotJSON)
		}
	}
	if len(reference.Parameters) != 4 {
		t.Fatalf("got %d parameters, want 4", len(reference.Parameters))
	}
}

func TestPermutationIgnoresAttributeOrder(t *testing.T) {
	a := `<fcpxml version="1.10"><resources><effect effectID="blur" name="Blur" id="r1"/></resources>` +
		`<clip><filter-video ref="r1"><param value="0.25" key="amount" name="Amount"/></filter-video></clip></fcpxml>`
	b := `<fcpxml version="1.10"><resources><effect id="r1" name="Blur" effectID="blur"/></resources>` +
		`<clip><filter-video ref="r1"><param name="Amount" key="amount" value="0.25"/></filter-video></clip></fcpxml>`
	left, err := ParseBytes([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	right, err := ParseBytes([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Snapshot(), right.Snapshot()) {
		t.Fatalf("attribute order changed snapshot:\n%#v\n%#v", left.Snapshot(), right.Snapshot())
	}
}

func TestLosslessSaveIsByteExact(t *testing.T) {
	input := []byte(`<fcpxml version="1.10"><resources><effect id="r1" name="Blur"/></resources><clip><filter-video ref="r1"><param name="Amount" value="1"/></filter-video></clip></fcpxml>`)
	doc, err := ParseBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := doc.Save(&out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), input) {
		t.Fatalf("save changed bytes:\n%s\n%s", input, out.Bytes())
	}
	path := filepath.Join(t.TempDir(), "out.fcpxml")
	if err := doc.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, input) {
		t.Fatalf("SaveFile changed bytes")
	}
}

func TestSameNamespaceValuesAreAllRetained(t *testing.T) {
	input := `<fcpxml version="1.10"><resources><effect id="r1" name="Blur"/></resources>` +
		`<clip><filter-video ref="r1"><param name="Amount" value="1"/><param name="Amount" value="2"/></filter-video></clip></fcpxml>`
	doc := parseTestXML(t, input)
	params := doc.Parameters()
	if len(params) != 2 {
		t.Fatalf("got %d parameters, want 2", len(params))
	}
	if params[0].Namespace() != params[1].Namespace() {
		t.Fatal("expected the exact same namespace in this collision test")
	}
	if params[0].Value.Text == params[1].Value.Text {
		t.Fatalf("duplicate values unexpectedly collapsed: %#v", params)
	}
	if doc.Snapshot().Digest == "" {
		t.Fatal("snapshot digest is empty")
	}
}

func TestSourceInNamespaceIsExplicit(t *testing.T) {
	doc := parseTestXML(t, `<fcpxml version="1.10"><clip><adjust-volume>`+
		`<param name="amount" value="-3dB"/></adjust-volume></clip></fcpxml>`)
	params := doc.Parameters()
	if len(params) != 1 {
		t.Fatalf("got %d parameters, want 1", len(params))
	}
	ns := params[0].Namespace()
	if !strings.Contains(ns, "adjust-volume") || !strings.Contains(ns, "param[amount]") {
		t.Fatalf("namespace %q does not expose effect type and source", ns)
	}
}
