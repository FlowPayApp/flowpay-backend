package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

const maxAttachmentBytes = 8 << 20 // 8 MiB

var allowedAttachmentExt = map[string]string{
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

func extFromFilename(name string) (extNoDot string, dotExt string, ok bool) {
	d := strings.ToLower(filepath.Ext(name))
	if d == "" {
		return "", "", false
	}
	mimeType, ok := allowedAttachmentExt[d]
	if !ok {
		return "", "", false
	}
	_ = mimeType
	extNoDot = strings.TrimPrefix(d, ".")
	return extNoDot, d, true
}

func mimeFromExt(extNoDot string) string {
	d := "." + strings.ToLower(strings.TrimPrefix(extNoDot, "."))
	if m, ok := allowedAttachmentExt[d]; ok {
		return m
	}
	return mime.TypeByExtension(d)
}

// SaveChargeAttachment guarda PDF/imagen y asocia el cobro.
func (s *Service) SaveChargeAttachment(ctx context.Context, companyID, chargeID, memberUID int64, src io.Reader, origName string, size int64) error {
	if s.UploadDir == "" {
		return errors.New("upload dir not configured")
	}
	extNoDot, _, ok := extFromFilename(origName)
	if !ok {
		return errors.New("solo se permiten PDF, PNG o JPG")
	}
	if size > maxAttachmentBytes {
		return errors.New("archivo demasiado grande (máx 8 MB)")
	}

	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return err
	}

	if ch.AttachmentToken != nil && *ch.AttachmentToken != "" && ch.AttachmentExt != nil {
		oldPath := filepath.Join(s.UploadDir, fmt.Sprintf("%s.%s", *ch.AttachmentToken, *ch.AttachmentExt))
		_ = os.Remove(oldPath)
	}

	token, err := randomToken()
	if err != nil {
		return err
	}
	dstPath := filepath.Join(s.UploadDir, fmt.Sprintf("%s.%s", token, extNoDot))
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(src, maxAttachmentBytes+1))
	out.Close()
	if err != nil {
		_ = os.Remove(dstPath)
		return err
	}
	if n > maxAttachmentBytes {
		_ = os.Remove(dstPath)
		return errors.New("archivo demasiado grande (máx 8 MB)")
	}

	if err := s.Repo.SetChargeAttachment(ctx, companyID, chargeID, token, extNoDot); err != nil {
		_ = os.Remove(dstPath)
		return err
	}
	return nil
}

// OpenPublicAttachment abre el archivo público por token (para GET y para verificar que existe).
func (s *Service) OpenPublicAttachment(ctx context.Context, token string) (*os.File, string, error) {
	if token == "" {
		return nil, "", errors.New("bad token")
	}
	ext, err := s.Repo.GetAttachmentExtByToken(ctx, token)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(s.UploadDir, fmt.Sprintf("%s.%s", token, ext))
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	return f, mimeFromExt(ext), nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
