package protocol

// PasteContent is sent only by the trusted Shell after an explicit paste.
// Exactly one of Text or Image must be present. Images are normalized to PNG.
type PasteContent struct {
	Text  *string     `json:"text,omitempty"`
	Image *PasteImage `json:"image,omitempty"`
}
type PasteImage struct {
	DataBase64 string `json:"dataBase64"`
}

// UILaunchImage identifies an immutable image accessible only in its UI session.
type UILaunchImage struct {
	URL      string `json:"url"`
	MIMEType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
}
