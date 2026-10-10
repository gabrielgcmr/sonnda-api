// internal/domain/demographics/identifiers.go
package demographics

// IsValidCPF validates the two CPF check digits and rejects repeated sequences.
// Formatting characters are ignored in the same way as CleanDigits.
func IsValidCPF(value string) bool {
	cpf := CleanDigits(value)
	if len(cpf) != 11 || allDigitsEqual(cpf) {
		return false
	}

	first := cpfCheckDigit(cpf[:9], 10)
	second := cpfCheckDigit(cpf[:9]+string(rune('0'+first)), 11)
	return int(cpf[9]-'0') == first && int(cpf[10]-'0') == second
}

func cpfCheckDigit(base string, initialWeight int) int {
	sum := 0
	for i := range base {
		sum += int(base[i]-'0') * (initialWeight - i)
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

// IsValidCNS implements both CNS algorithms published by ANS: the legacy
// PIS-based form and the provisional weighted modulo-11 form.
func IsValidCNS(value string) bool {
	cns := CleanDigits(value)
	if len(cns) != 15 || cns == "000000000000000" {
		return false
	}
	return validLegacyCNS(cns) || validProvisionalCNS(cns)
}

func validProvisionalCNS(cns string) bool {
	sum := 0
	for i := range cns {
		sum += int(cns[i]-'0') * (15 - i)
	}
	return sum%11 == 0
}

func validLegacyCNS(cns string) bool {
	pis := cns[:11]
	sum := 0
	for i := range pis {
		sum += int(pis[i]-'0') * (15 - i)
	}

	checkDigit := 11 - sum%11
	suffix := "000"
	switch checkDigit {
	case 11:
		checkDigit = 0
	case 10:
		sum += 2
		checkDigit = 11 - sum%11
		suffix = "001"
	}
	if checkDigit < 0 || checkDigit > 9 {
		return false
	}

	return cns == pis+suffix+string(rune('0'+checkDigit))
}

func allDigitsEqual(value string) bool {
	for i := 1; i < len(value); i++ {
		if value[i] != value[0] {
			return false
		}
	}
	return true
}
