package main

// schema.go defines the minimal re-importable object model. Anything in the
// input that falls outside this table is an "inexpressible property" and is
// either dropped (with a diagnostic) or, when structural, a hard failure.

type propRule struct {
	required bool // dropping this property invalidates the object
}

type classRule struct {
	props map[string]propRule
}

// minimalSchema lists the classes the minimal project can re-import.
// Property names follow common AAF naming (SMPTE ST 377-1 style).
var minimalSchema = map[string]classRule{
	"Header": {props: map[string]propRule{
		"ByteOrder":      {},
		"LastModified":   {},
		"ContentStorage": {required: true},
		"Dictionaries":   {},
	}},
	"ContentStorage": {props: map[string]propRule{
		"Mobs": {required: true},
	}},
	"CompositionMob": {props: map[string]propRule{
		"MobID":               {required: true},
		"Name":                {},
		"CreationTime":        {},
		"Slots":               {required: true},
		"DescriptiveMetadata": {},
	}},
	"MasterMob": {props: map[string]propRule{
		"MobID":               {required: true},
		"Name":                {},
		"CreationTime":        {},
		"Slots":               {required: true},
		"DescriptiveMetadata": {},
	}},
	"SourceMob": {props: map[string]propRule{
		"MobID":               {required: true},
		"Name":                {},
		"CreationTime":        {},
		"Slots":               {required: true},
		"EssenceDescriptor":   {},
		"DescriptiveMetadata": {},
	}},
	"TimelineMobSlot": {props: map[string]propRule{
		"SlotID":   {required: true},
		"SlotName": {},
		"EditRate": {required: true},
		"Segment":  {required: true},
	}},
	"Sequence": {props: map[string]propRule{
		"DataDefinition": {required: true},
		"Components":     {required: true},
	}},
	"SourceClip": {props: map[string]propRule{
		"DataDefinition": {required: true},
		"StartTime":      {required: true},
		"Length":         {required: true},
		"SourceID":       {required: true},
		"SourceMobSlot":  {},
	}},
	"Timecode": {props: map[string]propRule{
		"Start": {required: true},
		"FPS":   {required: true},
		"Drop":  {},
	}},
	"Filler": {props: map[string]propRule{
		"DataDefinition": {required: true},
		"Length":         {required: true},
	}},
	"OperationGroup": {props: map[string]propRule{
		"DataDefinition": {required: true},
		"Operation":      {required: true},
		"InputSegments":  {required: true},
		"Parameters":     {},
		"Length":         {},
	}},
	"OperationDefinition": {props: map[string]propRule{
		"Identification": {required: true},
		"Name":           {required: true},
		"DataDefinition": {},
	}},
	"Parameter": {props: map[string]propRule{
		"ParameterDefinition": {required: true},
		"Value":               {},
	}},
	"ParameterDefinition": {props: map[string]propRule{
		"Identification": {required: true},
		"Name":           {required: true},
	}},
	"DataDefinition": {props: map[string]propRule{
		"Identification": {required: true},
		"Name":           {required: true},
	}},
	"CDCIDescriptor": {props: map[string]propRule{
		"EssenceKind":   {required: true},
		"SampleRate":    {required: true},
		"StoredWidth":   {},
		"StoredHeight":  {},
		"ComponentSize": {},
	}},
	"WaveDescriptor": {props: map[string]propRule{
		"EssenceKind": {required: true},
		"SampleRate":  {required: true},
		"Channels":    {},
		"BitDepth":    {},
	}},
	"DMFramework": {props: map[string]propRule{
		"Scheme": {required: true},
	}},
	"DMSet": {props: map[string]propRule{
		"Scheme": {},
	}},
}

// effectClasses are classes whose absence must never be silently treated as
// "no effect" — the failure mode of the existing converter.
var effectClasses = map[string]bool{
	"OperationDefinition": true,
	"ParameterDefinition": true,
	"Parameter":           true,
}

func classExpressible(class string) bool {
	_, ok := minimalSchema[class]
	return ok
}
