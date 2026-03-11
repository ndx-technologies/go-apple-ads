package analysis

import "strings"

// Info is a non-error result of analyis that can represent itself as a short oneliner text.
type Info interface{ String() string }

type CompositeInfo struct{ Infos []Info }

func (c CompositeInfo) String() string {
	var b strings.Builder
	for _, i := range c.Infos {
		b.WriteString(i.String())
		b.WriteString("; ")
	}
	return b.String()
}

func Join(vs ...Info) CompositeInfo { return CompositeInfo{Infos: vs} }
