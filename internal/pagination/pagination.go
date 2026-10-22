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

// contract-copy: fallback for module callers that have no request; the contract declares the same default
const DefaultPageSize = 25

type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

func Normalize(page, pageSize *int32) (int, int) {
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
