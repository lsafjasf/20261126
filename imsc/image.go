package imsc

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// supportedImageTypes maps normalised smpte:imageType values to the format
// name reported by the stdlib image sniffers.
var supportedImageTypes = map[string]string{
	"png":  "png",
	"jpeg": "jpeg",
	"jpg":  "jpeg",
}

// processImages validates every smpte:image element. Failures are returned as
// element errors and are excluded from the successful image list, so a broken
// resource can never be treated as a usable image.
func processImages(root *node) ([]*Image, ErrorList) {
	var images []*Image
	var errs ErrorList
	seen := map[string]bool{}

	for _, n := range root.collect("image") {
		path := n.path

		id, ok := n.attr("id")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			errs.append(NewElementError(path, n.Line, ReasonMissingAttr,
				"smpte:image requires a non-empty xml:id"))
			continue
		}
		if seen[id] {
			errs.append(NewElementError(path, n.Line, ReasonDuplicateID,
				"duplicate smpte:image xml:id "+id))
			continue
		}

		// IMSC documents use smpte:imageType; accept the historical
		// smpte:type shorthand as well.
		imageType, ok := n.attr("imageType")
		if !ok {
			imageType, ok = n.attr("type")
		}
		imageType = strings.TrimSpace(imageType)
		if !ok || imageType == "" {
			errs.append(NewElementError(path, n.Line, ReasonMissingAttr,
				"smpte:image "+id+" requires smpte:imageType"))
			continue
		}

		encoding, ok := n.attr("encoding")
		encoding = strings.TrimSpace(encoding)
		if !ok || encoding == "" {
			errs.append(NewElementError(path, n.Line, ReasonMissingAttr,
				"smpte:image "+id+" requires smpte:encoding"))
			continue
		}
		if !strings.EqualFold(encoding, "Base64") {
			errs.append(NewElementError(path, n.Line, ReasonBadEncoding,
				"smpte:image "+id+" has unsupported encoding "+encoding+"; only Base64 is supported"))
			continue
		}

		normalized := strings.ToLower(imageType)
		wantFormat, supported := supportedImageTypes[normalized]
		if !supported {
			errs.append(NewElementError(path, n.Line, ReasonUnsupportedImage,
				"smpte:image "+id+" has unsupported imageType "+imageType+"; only PNG and JPEG are supported"))
			continue
		}

		payload := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, n.Text)
		if payload == "" {
			errs.append(NewElementError(path, n.Line, ReasonEmptyAttr,
				"smpte:image "+id+" contains no Base64 payload"))
			continue
		}

		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			errs.append(NewElementError(path, n.Line, ReasonBadBase64,
				"smpte:image "+id+" payload is not valid Base64: "+err.Error()))
			continue
		}

		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			errs.append(NewElementError(path, n.Line, ReasonUnreadableImage,
				"smpte:image "+id+" cannot be decoded as "+wantFormat+": "+err.Error()))
			continue
		}
		if format != wantFormat {
			errs.append(NewElementError(path, n.Line, ReasonUnreadableImage,
				"smpte:image "+id+" declares "+imageType+" but payload is "+format))
			continue
		}
		if cfg.Width <= 0 || cfg.Height <= 0 {
			errs.append(NewElementError(path, n.Line, ReasonUnreadableImage,
				"smpte:image "+id+" has non-positive dimensions"))
			continue
		}

		seen[id] = true
		images = append(images, &Image{
			ID:   id,
			Type: normalized,
			Data: data,
			Line: n.Line,
		})
	}

	return images, errs
}
