package entity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validCommentInput() NewTemplateCommentInput {
	return NewTemplateCommentInput{
		TemplateID:        "tpl-1",
		TemplateVersionID: "ver-1",
		Audience:          "AYELEN",
		Body:              "La cláusula 3 no coincide con el original.",
		AuthorEmail:       "director@ayelen.cl",
		AuthorName:        "Directora",
		AuthorCampus:      "AYELEN",
	}
}

func TestNewTemplateCommentTrimsFields(t *testing.T) {
	t.Parallel()

	in := validCommentInput()
	in.Audience = "  AYELEN "
	in.Body = "\n  texto  \t"
	in.AuthorEmail = " director@ayelen.cl "
	in.AuthorName = " Directora "
	in.AuthorCampus = " AYELEN "

	c, err := NewTemplateComment(in)
	require.NoError(t, err)
	assert.Equal(t, "AYELEN", c.Audience)
	assert.Equal(t, "texto", c.Body)
	assert.Equal(t, "director@ayelen.cl", c.AuthorEmail)
	require.NotNil(t, c.AuthorName)
	assert.Equal(t, "Directora", *c.AuthorName)
	assert.Equal(t, "AYELEN", c.AuthorCampus)
	assert.Equal(t, "tpl-1", c.TemplateID)
	assert.Equal(t, "ver-1", c.TemplateVersionID)
}

func TestNewTemplateCommentEmptyNameIsNil(t *testing.T) {
	t.Parallel()

	in := validCommentInput()
	in.AuthorName = "   "

	c, err := NewTemplateComment(in)
	require.NoError(t, err)
	assert.Nil(t, c.AuthorName)
}

func TestNewTemplateCommentRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*NewTemplateCommentInput){
		"empty body":       func(in *NewTemplateCommentInput) { in.Body = "" },
		"whitespace body":  func(in *NewTemplateCommentInput) { in.Body = " \n\t " },
		"body too long":    func(in *NewTemplateCommentInput) { in.Body = strings.Repeat("a", TemplateCommentMaxLength+1) },
		"missing audience": func(in *NewTemplateCommentInput) { in.Audience = " " },
		"missing email":    func(in *NewTemplateCommentInput) { in.AuthorEmail = "" },
		"missing campus":   func(in *NewTemplateCommentInput) { in.AuthorCampus = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := validCommentInput()
			mutate(&in)
			_, err := NewTemplateComment(in)
			assert.ErrorIs(t, err, ErrTemplateCommentInvalid)
		})
	}
}

// The limit is in characters, as char_length counts them in Postgres, not bytes.
func TestNewTemplateCommentCountsCharactersNotBytes(t *testing.T) {
	t.Parallel()

	in := validCommentInput()
	in.Body = strings.Repeat("ñ", TemplateCommentMaxLength)
	_, err := NewTemplateComment(in)
	require.NoError(t, err, "4000 multibyte characters are within the limit")

	in.Body = strings.Repeat("ñ", TemplateCommentMaxLength+1)
	_, err = NewTemplateComment(in)
	assert.ErrorIs(t, err, ErrTemplateCommentInvalid)
}

// Trimming happens before the length check, so surrounding whitespace never
// pushes an otherwise valid body over the limit.
func TestNewTemplateCommentLengthIsMeasuredAfterTrim(t *testing.T) {
	t.Parallel()

	in := validCommentInput()
	in.Body = "  " + strings.Repeat("a", TemplateCommentMaxLength) + "  "
	c, err := NewTemplateComment(in)
	require.NoError(t, err)
	assert.Len(t, c.Body, TemplateCommentMaxLength)
}
