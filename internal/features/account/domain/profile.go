// internal/features/account/domain/profile.go
package accountdomain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"
)

var (
	ErrInvalidBirthDate = errors.New("invalid birth date")
	ErrInvalidFullName  = errors.New("invalid full name")
	ErrInvalidPhone     = errors.New("invalid phone")
	ErrInvalidCPF       = errors.New("invalid cpf")
	phonePattern        = regexp.MustCompile(`^\+?[0-9]{10,15}$`)
)

type Profile struct {
	FullName  *string    `json:"full_name"`
	BirthDate *time.Time `json:"birth_date"`
	CPF       *string    `json:"cpf"`
	Phone     *string    `json:"phone"`
}

// Normalize returns owned values; it never changes the caller's pointers.
func (p Profile) Normalize() Profile {
	next := Profile{FullName: trimmed(p.FullName, false), Phone: trimmed(p.Phone, true)}
	if p.BirthDate != nil {
		date := calendarDate(*p.BirthDate)
		next.BirthDate = &date
	}
	if value := trimmed(p.CPF, true); value != nil {
		digits := demographics.CleanDigits(*value)
		next.CPF = &digits
	}
	return next
}

func (p Profile) Validate() error {
	if p.FullName != nil && !validName(*p.FullName) {
		return ErrInvalidFullName
	}
	if p.BirthDate != nil && !validBirthDate(*p.BirthDate) {
		return ErrInvalidBirthDate
	}
	if p.CPF != nil && !demographics.IsValidCPF(*p.CPF) {
		return ErrInvalidCPF
	}
	if p.Phone != nil && !phonePattern.MatchString(*p.Phone) {
		return ErrInvalidPhone
	}
	return nil
}

func (p Profile) OnboardingCompleted() bool {
	return p.FullName != nil && validName(*p.FullName) && p.BirthDate != nil && validBirthDate(*p.BirthDate)
}

func (p Profile) Equal(other Profile) bool {
	return sameString(p.FullName, other.FullName) && sameString(p.CPF, other.CPF) &&
		sameString(p.Phone, other.Phone) && sameDate(p.BirthDate, other.BirthDate)
}

func validName(value string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(value))
	return n >= 2 && n <= 120
}

func validBirthDate(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999 &&
		!calendarDate(value).After(calendarDate(time.Now().UTC()))
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func trimmed(value *string, emptyAsNil bool) *string {
	if value == nil {
		return nil
	}
	next := strings.TrimSpace(*value)
	if emptyAsNil && next == "" {
		return nil
	}
	return &next
}

func sameString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func sameDate(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && calendarDate(*a).Equal(calendarDate(*b))
}
