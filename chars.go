package cea608

// specialChars maps 0x11/0x19 + 0x30..0x3F to the CEA-608 special set.
var specialChars = [16]rune{
	'®', '°', '½', '¿', '™', '¢', '£', '♪',
	'à', ' ', 'è', 'â', 'ê', 'î', 'ô', 'û',
}

// extendedChars contains the Western/Spanish/French and Portuguese/German
// extension sets selected by prefix 0x12/0x1A and 0x13/0x1B.
var extendedChars = [2][16]rune{
	{
		'À', 'Â', 'Ç', 'È', 'Ê', 'Ë', 'ë', 'Î',
		'Ï', 'ï', 'Ô', 'Ù', 'ù', 'Û', '«', '»',
	},
	{
		'Ã', 'ã', 'Í', 'Ì', 'ì', 'Ò', 'ò', 'Õ',
		'õ', '{', '}', '\\', '^', '_', '|', '~',
	},
}

func decodeSpecial(b2 byte) rune {
	if b2 < 0x30 || b2 > 0x3f {
		return 0
	}
	return specialChars[b2-0x30]
}

func decodeExtended(b1, b2 byte) rune {
	if b2 < 0x30 || b2 > 0x3f {
		return 0
	}
	set := 0
	if normalizeControlFirst(b1) == 0x13 {
		set = 1
	}
	return extendedChars[set][b2-0x30]
}
