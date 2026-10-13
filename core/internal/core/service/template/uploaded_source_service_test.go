package template

import (
	"bytes"
	"context"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

func TestStorePDFAndPlaceFields(t *testing.T) {
	version := &entity.TemplateVersion{ID: "ver-1", TemplateID: "tpl-1", Status: entity.VersionStatusDraft}
	sources := &memSources{}
	storage := &memStorage{}
	svc := NewUploadedSourceService(&memVersions{version: version}, &memRoles{}, sources, storage)

	pdf := []byte("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
		"2 0 obj << /Type /Pages /Count 1 /Kids [3 0 R] >> endobj\n" +
		"3 0 obj << /Type /Page /MediaBox [0 0 200 400] >> endobj\n%%EOF")
	stored, err := svc.StorePDF(context.Background(), templateuc.StoreUploadedPDFCommand{TemplateID: "tpl-1", VersionID: "ver-1", PDF: pdf})
	if err != nil {
		t.Fatal(err)
	}
	if stored.PageCount != 1 || stored.PageSizes[0].Width != 200 {
		t.Fatalf("stored = %+v", stored)
	}
	if !bytes.Equal(storage.data, pdf) {
		t.Fatal("pdf bytes were not stored")
	}

	placed, err := svc.ReplacePlacements(context.Background(), templateuc.ReplacePlacementsCommand{
		TemplateID: "tpl-1",
		VersionID:  "ver-1",
		Signers: []templateuc.SignerPlacement{{
			Name:  "Ada",
			Order: 1,
			Fields: []entity.PlacedField{{
				Type: "signature", Page: 1, X: 10, Y: 20, Width: 50, Height: 20,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if placed.Signers[0].RoleID != "role-1" || sources.fields[0].Type != "SIGNATURE" || sources.fields[0].RoleID != "role-1" {
		t.Fatalf("placed = %+v fields = %+v", placed, sources.fields)
	}
}

func TestReplacePlacementsRejectsBoxOutsidePage(t *testing.T) {
	version := &entity.TemplateVersion{ID: "ver-1", TemplateID: "tpl-1", Status: entity.VersionStatusDraft}
	sources := &memSources{source: &entity.UploadedSource{
		VersionID: "ver-1",
		PageSizes: []entity.PageSize{{Width: 200, Height: 400}},
	}}
	svc := NewUploadedSourceService(&memVersions{version: version}, &memRoles{}, sources, &memStorage{})
	_, err := svc.ReplacePlacements(context.Background(), templateuc.ReplacePlacementsCommand{
		TemplateID: "tpl-1",
		VersionID:  "ver-1",
		Signers: []templateuc.SignerPlacement{{
			Name: "Ada", Order: 1,
			Fields: []entity.PlacedField{{Type: "SIGNATURE", Page: 1, X: 180, Y: 20, Width: 50, Height: 20}},
		}},
	})
	if err == nil {
		t.Fatal("expected box to be rejected")
	}
}

type memVersions struct{ version *entity.TemplateVersion }

func (m *memVersions) FindByID(context.Context, string) (*entity.TemplateVersion, error) {
	return m.version, nil
}

type memRoles struct{ n int }

func (m *memRoles) DeleteByVersionID(context.Context, string) error { return nil }
func (m *memRoles) Create(_ context.Context, _ *entity.TemplateVersionSignerRole) (string, error) {
	m.n++
	return "role-1", nil
}

type memSources struct {
	source *entity.UploadedSource
	fields []entity.PlacedField
}

func (m *memSources) UpsertPDF(_ context.Context, source *entity.UploadedSource) error {
	m.source = source
	return nil
}
func (m *memSources) ReplaceFields(_ context.Context, _ string, fields []entity.PlacedField) error {
	m.fields = fields
	return nil
}
func (m *memSources) FindByVersionID(context.Context, string) (*entity.UploadedSource, error) {
	if m.source == nil {
		return nil, entity.ErrUploadedSourceNotFound
	}
	return m.source, nil
}

type memStorage struct{ data []byte }

func (m *memStorage) Upload(_ context.Context, req *port.StorageUploadRequest) error {
	m.data = append([]byte(nil), req.Data...)
	return nil
}
func (m *memStorage) Download(context.Context, *port.StorageRequest) ([]byte, error) {
	return m.data, nil
}
func (m *memStorage) GetURL(context.Context, *port.StorageRequest) (string, error) { return "", nil }
func (m *memStorage) Delete(context.Context, *port.StorageRequest) error           { return nil }
func (m *memStorage) Exists(context.Context, *port.StorageRequest) (bool, error)   { return true, nil }
