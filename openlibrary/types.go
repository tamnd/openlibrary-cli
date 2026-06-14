package openlibrary

// Book is one book record from Open Library.
type Book struct {
	Rank        int      `json:"rank,omitempty"               csv:"rank"               tsv:"rank"`
	Key         string   `json:"key,omitempty"                csv:"key"                tsv:"key"`
	Title       string   `json:"title"                        csv:"title"              tsv:"title"`
	Authors     []string `json:"authors"                      csv:"authors"            tsv:"authors"`
	Publishers  []string `json:"publishers,omitempty"         csv:"publishers"         tsv:"publishers"`
	PublishDate string   `json:"publish_date,omitempty"       csv:"publish_date"       tsv:"publish_date"`
	FirstYear   int      `json:"first_publish_year,omitempty" csv:"first_publish_year" tsv:"first_publish_year"`
	Editions    int      `json:"edition_count,omitempty"      csv:"edition_count"      tsv:"edition_count"`
	EbookAccess string   `json:"ebook_access,omitempty"       csv:"ebook_access"       tsv:"ebook_access"`
	Languages   []string `json:"languages,omitempty"          csv:"languages"          tsv:"languages"`
	CoverID     int      `json:"cover_id,omitempty"           csv:"cover_id"           tsv:"cover_id"`
	CoverURL    string   `json:"cover_url,omitempty"          csv:"cover_url"          tsv:"cover_url"`
	Pages       int      `json:"pages,omitempty"              csv:"pages"              tsv:"pages"`
	Subjects    []string `json:"subjects,omitempty"           csv:"subjects"           tsv:"subjects"`
	URL         string   `json:"url,omitempty"                csv:"url"                tsv:"url"`
}

// Subject is a subject category with its book count.
type Subject struct {
	Rank      int    `json:"rank"       csv:"rank"       tsv:"rank"`
	Name      string `json:"name"       csv:"name"       tsv:"name"`
	WorkCount int    `json:"work_count" csv:"work_count" tsv:"work_count"`
	URL       string `json:"url"        csv:"url"        tsv:"url"`
}

// Author is one author record from Open Library.
type Author struct {
	Rank      int    `json:"rank"        csv:"rank"        tsv:"rank"`
	Key       string `json:"key"         csv:"key"         tsv:"key"`
	Name      string `json:"name"        csv:"name"        tsv:"name"`
	BirthDate string `json:"birth_date"  csv:"birth_date"  tsv:"birth_date"`
	DeathDate string `json:"death_date"  csv:"death_date"  tsv:"death_date"`
	WorkCount int    `json:"work_count"  csv:"work_count"  tsv:"work_count"`
	TopWork   string `json:"top_work"    csv:"top_work"    tsv:"top_work"`
	URL       string `json:"url"         csv:"url"         tsv:"url"`
}

// Work is one work/book record from the /works/{OLID}.json endpoint.
type Work struct {
	Key        string   `json:"key"         csv:"key"         tsv:"key"`
	Title      string   `json:"title"       csv:"title"       tsv:"title"`
	Desc       string   `json:"description" csv:"description" tsv:"description"`
	Subjects   []string `json:"subjects"    csv:"subjects"    tsv:"subjects"`
	AuthorKeys []string `json:"author_keys" csv:"author_keys" tsv:"author_keys"`
	URL        string   `json:"url"         csv:"url"         tsv:"url"`
}
