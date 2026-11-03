package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// maxSendPDFBytes is the largest PDF "Send a PDF" accepts, the same limit as
// the automation API's source-document upload.
const maxSendPDFBytes = 12 << 20

// maxSendPDFRequestBytes bounds the whole multipart body: the PDF, the JSON
// payload and the form's framing.
const maxSendPDFRequestBytes = maxSendPDFBytes + 1<<20

// SendPDF creates a document from an uploaded PDF and sends it.
// @Summary Send a PDF for signature
// @Description Multipart form with "file" (the PDF, at most 12 MiB) and "payload" (JSON: title, and signers with name, email, order and boxes in PDF points from the page's bottom-left). Builds a one-off template, publishes it and creates the document; with signing.direct_send on, the provider emails the signers. Registered only when signing.pdf_send is on.
// @Tags Documents
// @Accept multipart/form-data
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param file formData file true "The PDF"
// @Param payload formData string true "JSON: title and signers"
// @Success 201 {object} entity.DocumentWithRecipients
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/documents/from-pdf [post]
func (c *DocumentController) SendPDF(ctx *gin.Context) {
	workspaceID, ok := middleware.GetWorkspaceID(ctx)
	if !ok || strings.TrimSpace(workspaceID) == "" {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}
	userID, _ := middleware.GetInternalUserID(ctx)

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxSendPDFRequestBytes)
	pdf, err := readSendPDFFile(ctx)
	if err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	var req dto.SendPDFRequest
	if err := json.Unmarshal([]byte(ctx.PostForm("payload")), &req); err != nil {
		respondError(ctx, http.StatusBadRequest, fmt.Errorf("payload: %w", err))
		return
	}

	signers := make([]documentuc.PDFSigner, 0, len(req.Signers))
	for _, signer := range req.Signers {
		fields := make([]entity.PlacedField, 0, len(signer.Fields))
		for _, field := range signer.Fields {
			fields = append(fields, entity.PlacedField{
				Type: field.Type, Page: field.Page, X: field.X, Y: field.Y, Width: field.Width, Height: field.Height,
			})
		}
		signers = append(signers, documentuc.PDFSigner{
			Name:   signer.Name,
			Email:  signer.Email,
			Order:  signer.Order,
			Fields: fields,
		})
	}

	doc, err := c.pdfSendUC.SendPDF(ctx.Request.Context(), documentuc.SendPDFCommand{
		WorkspaceID:  workspaceID,
		UserID:       userID,
		Title:        req.Title,
		PDF:          pdf,
		Signers:      signers,
		SigningEmail: signingEmailCommand(req.SigningEmail),
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, doc)
}

func readSendPDFFile(ctx *gin.Context) ([]byte, error) {
	file, err := ctx.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	if file.Size > maxSendPDFBytes {
		return nil, fmt.Errorf("file: the PDF is larger than %d MiB", maxSendPDFBytes>>20)
	}
	opened, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	defer opened.Close()
	return io.ReadAll(io.LimitReader(opened, maxSendPDFBytes+1))
}

// signingEmailCommand maps the optional request field; nil stays nil.
func signingEmailCommand(req *dto.SigningEmailRequest) *entity.SigningEmail {
	if req == nil {
		return nil
	}
	return &entity.SigningEmail{Subject: req.Subject, Message: req.Message}
}
