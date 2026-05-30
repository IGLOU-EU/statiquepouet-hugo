// Hugo module: ships only the StatiquePouet partials under layouts/.
// The announce/ tool is a separate nested Go module (its own go.mod), so
// it is excluded here and not fetched by `hugo mod get`. Templates carry
// no Go dependencies, hence no require block.
module gitlab.com/iglou.eu/statiquepouet-hugo

go 1.21
