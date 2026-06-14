package openlibrary

// Book is a search result record from Open Library.
type Book struct {
	Key         string   `kit:"id" json:"key"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	PublishYear int      `json:"first_publish_year"`
	ISBN        []string `json:"isbn"`
}

// Work is a full work record from /works/{key}.json.
type Work struct {
	Key         string   `kit:"id" json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Subjects    []string `json:"subjects"`
	Covers      []int    `json:"covers"`
}

// Author is an author record from /authors/{key}.json.
type Author struct {
	Key       string `kit:"id" json:"key"`
	Name      string `json:"name"`
	BirthDate string `json:"birth_date"`
	DeathDate string `json:"death_date"`
	Bio       string `json:"bio"`
	WorkCount int    `json:"work_count"`
}

// SubjectWork is one work entry from a subject listing.
type SubjectWork struct {
	Key     string   `kit:"id" json:"key"`
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
}

// Edition is a book edition record from /isbn/{isbn}.json.
type Edition struct {
	Key         string   `kit:"id" json:"key"`
	Title       string   `json:"title"`
	Publishers  []string `json:"publishers"`
	PublishDate string   `json:"publish_date"`
	Pages       int      `json:"pages"`
	ISBN10      []string `json:"isbn_10"`
	ISBN13      []string `json:"isbn_13"`
}
