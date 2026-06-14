package openlibrary

// Book is a search result or ISBN lookup record from Open Library.
type Book struct {
	Key              string `kit:"id" json:"key"`
	Title            string `json:"title"`
	Authors          string `json:"authors"`            // comma-joined author names
	FirstPublishYear int    `json:"first_publish_year"`
	ISBN             string `json:"isbn"`               // first ISBN if any
	Subjects         string `json:"subjects"`           // comma-joined first 3 subjects
	Pages            int    `json:"pages"`              // number_of_pages_median (search) or number_of_pages (isbn)
}

// Work is a full work record from /works/{key}.json.
type Work struct {
	Key          string `kit:"id" json:"key"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Subjects     string `json:"subjects"`      // comma-joined first 5 subjects
	FirstPublish string `json:"first_publish"`
}

// Author is an author record from /authors/{key}.json.
type Author struct {
	Key       string `kit:"id" json:"key"`
	Name      string `json:"name"`
	BirthDate string `json:"birth_date"`
	DeathDate string `json:"death_date"`
	Bio       string `json:"bio"`
}

// Edition is one edition entry from /works/{key}/editions.json.
type Edition struct {
	Key       string `kit:"id" json:"key"`
	Title     string `json:"title"`
	Publisher string `json:"publisher"` // first publisher
	Published string `json:"published"`
	ISBN13    string `json:"isbn_13"`   // first isbn_13
}
