package creation

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func (s *Service) attachDiagram(p *Snapshot, c Command) (commandOutcome, error) {
	image, err := diagramImage(c.Diagram)
	if err != nil {
		return commandOutcome{}, err
	}
	at := len(p.Messages)
	if err := s.attachNote(p, c.Message); err != nil {
		return commandOutcome{}, err
	}
	h := sha256.Sum256(image)
	p.DiagramFingerprint = hex.EncodeToString(h[:])
	p.DiagramMediaType = c.Diagram.MediaType
	p.DiagramBytes = len(image)
	p.Attachments = append(p.Attachments, Attachment{
		MessageIndex: at,
		MediaType:    p.DiagramMediaType,
		Bytes:        p.DiagramBytes,
		SHA256:       p.DiagramFingerprint,
	})
	p.DiagramUnderstanding = ""
	p.DiagramDescription = ""
	p.DiagramDescriptionConfirmed = false
	p.DiagramInterpretation = nil
	p.DiagramConfirmed = false
	p.BriefConfirmed = false
	invalidate(p)
	return commandOutcome{queueStep: true, transient: true}, nil
}

func diagramImage(d *Diagram) ([]byte, error) {
	if d == nil {
		return nil, ErrInvalidCommand
	}
	image, err := base64.StdEncoding.DecodeString(d.Data)
	if err != nil || len(image) == 0 || len(image) > MaxDiagramBytes {
		return nil, ErrInvalidCommand
	}
	switch d.MediaType {
	case "image/png", "image/jpeg", "image/webp":
		return image, nil
	}
	return nil, ErrInvalidCommand
}
