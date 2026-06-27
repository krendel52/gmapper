package api

//go:generate go run github.com/krendel52/gmapper -from github.com/krendel52/gmapper/example/domain.DomainModel -to ApiModel

type ApiModel struct {
	ID     int64
	Name   string
	Count  int
	Extra2 string
}
