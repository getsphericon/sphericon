// Package pagination is the one page shape of every module List.
//
// Order rule: each module owns one order, used by both API surfaces. History
// (Broadcasts, Automations runs, events) lists newest first, by id descending;
// catalogues (Templates, Segments, Tags, ...) list ascending by id.
package pagination

import "context"

// DefaultPage and DefaultPageSize are what a module caller without a request gets. The
// contract (PageQuery) declares the same defaults and the page size maximum, which ogen
// enforces before a handler runs.
const DefaultPage = 1

// DefaultPageSize is the fallback for module callers that have no request.
// contract-copy: the contract declares the same default
const DefaultPageSize = 25

type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

func Normalize(page, pageSize *int32) (normalPage, normalPageSize int) {
	p := DefaultPage
	ps := DefaultPageSize
	if page != nil && *page > 0 {
		p = int(*page)
	}
	if pageSize != nil && *pageSize > 0 {
		ps = int(*pageSize)
	}
	return p, ps
}

func Offset(page, pageSize int) int {
	return (page - 1) * pageSize
}

func TotalPages(totalItems, pageSize int) int {
	if pageSize == 0 {
		return 0
	}
	total := totalItems / pageSize
	if totalItems%pageSize > 0 {
		total++
	}
	return total
}

// Paginate returns the requested page of an in-memory slice (for small catalogues
// that are cheaper to load whole than to page in SQL). Items is never nil.
func Paginate[T any](items []T, page, pageSize *int32) Page[T] {
	p, ps := Normalize(page, pageSize)
	start := min(Offset(p, ps), len(items))
	end := min(start+ps, len(items))
	return Page[T]{
		Items:      append([]T{}, items[start:end]...),
		Page:       p,
		PageSize:   ps,
		TotalItems: len(items),
		TotalPages: TotalPages(len(items), ps),
	}
}

// Params are normalized page params: Page >= 1, PageSize >= 1.
type Params struct {
	Page     int
	PageSize int
}

// Offset is the number of rows before the page.
func (p Params) Offset() int { return Offset(p.Page, p.PageSize) }

// optional is satisfied by the generated optional integer page params of every surface.
type optional interface{ Get() (int32, bool) }

// ParamsOf normalizes the generated optional page params, applying the defaults.
func ParamsOf(page, pageSize optional) Params {
	var pp, sp *int32
	if v, ok := page.Get(); ok {
		pp = &v
	}
	if v, ok := pageSize.Get(); ok {
		sp = &v
	}
	p, s := Normalize(pp, sp)
	return Params{Page: p, PageSize: s}
}

// List builds one page from two closures: the total count, and the rows for a
// limit and offset. Ent builders share no interface, hence closures. Items is never nil.
func List[T any](
	ctx context.Context,
	p Params,
	count func(context.Context) (int, error),
	fetch func(ctx context.Context, limit, offset int) ([]T, error),
) (Page[T], error) {
	total, err := count(ctx)
	if err != nil {
		return Page[T]{}, err
	}
	items, err := fetch(ctx, p.PageSize, p.Offset())
	if err != nil {
		return Page[T]{}, err
	}
	if items == nil {
		items = []T{}
	}
	return Page[T]{
		Items:      items,
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalItems: total,
		TotalPages: TotalPages(total, p.PageSize),
	}, nil
}

// Map is the page with each item passed through f: the handler's own mapping of the items (an
// extra a Converter cannot attach), before the page goes to the Converter. Items is never nil.
func Map[T, U any](p Page[T], f func(T) U) Page[U] {
	items := make([]U, len(p.Items))
	for i, item := range p.Items {
		items[i] = f(item)
	}
	return Page[U]{Items: items, Page: p.Page, PageSize: p.PageSize, TotalItems: p.TotalItems, TotalPages: p.TotalPages}
}

// MapErr is Map with a mapping that can fail: the first error is returned and no page.
func MapErr[T, U any](p Page[T], f func(T) (U, error)) (Page[U], error) {
	items := make([]U, len(p.Items))
	for i, item := range p.Items {
		var err error
		if items[i], err = f(item); err != nil {
			return Page[U]{}, err
		}
	}
	return Page[U]{Items: items, Page: p.Page, PageSize: p.PageSize, TotalItems: p.TotalItems, TotalPages: p.TotalPages}, nil
}

// Whole is an unpaginated list as its one page: page 1 of 1, as large as the list. Items is
// never nil.
func Whole[T any](items []T) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Page: DefaultPage, PageSize: len(items), TotalItems: len(items), TotalPages: 1}
}

// CursorPage is one page of a feed read after a cursor: the items, the cursor to read what
// follows, and whether more items follow right now. A page method of a Converter maps it whole,
// as it maps a Page.
type CursorPage[T any] struct {
	Items []T
	// NextCursor is the cursor after the last item, or the requested one on an empty page.
	NextCursor string
	// HasMore reports whether items follow the page.
	HasMore bool
}

// Cursor is the page of the rows a feed read for a limit: up to limit+1 rows in feed order, one
// past the limit meaning that more follow. NextCursor is cursorOf the last row kept, or the
// requested cursor when no row is. Items is never nil.
func Cursor[T any](rows []T, limit int, requested string, cursorOf func(T) string) CursorPage[T] {
	items := append([]T{}, rows[:min(len(rows), limit)]...)
	page := CursorPage[T]{Items: items, NextCursor: requested, HasMore: len(rows) > limit}
	if n := len(items); n > 0 {
		page.NextCursor = cursorOf(items[n-1])
	}
	return page
}

// MapCursor is Map for a CursorPage: the cursor kept, each item passed through f (an extra a
// Converter cannot attach). Items is never nil.
func MapCursor[T, U any](p CursorPage[T], f func(T) U) CursorPage[U] {
	items := make([]U, len(p.Items))
	for i, item := range p.Items {
		items[i] = f(item)
	}
	return CursorPage[U]{Items: items, NextCursor: p.NextCursor, HasMore: p.HasMore}
}
