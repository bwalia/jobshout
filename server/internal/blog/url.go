package blog

import "strings"

// PublicImageURL turns a stored cover path into an absolute URL opsapi (and
// any browser outside JobShout) can load. Exported so a regenerated cover can
// be pushed to an existing CMS post with the same absolute URL Publish uses.
//
// Covers are stored relative to this host — /api/v1/images/file/… — so they
// survive a ring being reached on different hostnames. opsapi's Featured image
// field is a plain URL string rendered with <img>, so a relative path would
// resolve against the opsapi console origin and 404. When base is empty the
// cover is omitted rather than sending a broken path.
func PublicImageURL(base, imageURL string) string {
	return publicImageURL(base, imageURL)
}

func publicImageURL(base, imageURL string) string {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return ""
	}
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(imageURL, "/") {
		imageURL = "/" + imageURL
	}
	return base + imageURL
}
