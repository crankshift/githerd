package tui

// railKey handles a key on the rail.
func railKey(m Model, k Key) (Model, []Cmd) {
	if m.filtering {
		return filterKey(m, k), nil
	}
	switch k {
	case "j", KeyDown:
		m = m.move(1)
	case "k", KeyUp:
		m = m.move(-1)
	case KeyEnter, "l", KeyRight:
		return m.openPage()
	case "/":
		m.filtering = true
	case KeyEsc:
		m.filter = ""
	default:
		return sharedKey(m, k)
	}
	return m, nil
}

// sharedKey handles the keys that do the same on the rail and on the repo
// page: z zooms the rail's pane in or out, f fetches every repo of the
// root, o, s and L open the selected repo, and q quits.
func sharedKey(m Model, k Key) (Model, []Cmd) {
	switch k {
	case "z":
		return m, m.zoom(ZoomToggle)
	case "f":
		return m.forceFetch()
	case "o", "s", "L":
		return act(m, k)
	case "q":
		return m, []Cmd{Quit{}}
	}
	return m, nil
}

// filterKey handles a key while the filter is being typed: a character
// extends it, Backspace shortens it, Enter keeps it and Esc clears it. The
// arrows still move.
func filterKey(m Model, k Key) Model {
	switch k {
	case KeyEnter:
		m.filtering = false
	case KeyEsc:
		m.filtering, m.filter = false, ""
	case KeyBackspace:
		if runes := []rune(m.filter); len(runes) > 0 {
			m.filter = string(runes[:len(runes)-1])
		}
	case KeyUp:
		m = m.move(-1)
	case KeyDown:
		m = m.move(1)
	default:
		if r := []rune(k); len(r) == 1 && r[0] >= ' ' {
			m.filter += string(k)
		}
	}
	return m
}

// pageKey handles a key on the repo page.
func pageKey(m Model, k Key) (Model, []Cmd) {
	switch k {
	case KeyEsc, "h", KeyLeft:
		m.page = page{}
		return m, m.zoom(ZoomOff)
	case "j", KeyDown:
		m.page = m.page.move(1)
	case "k", KeyUp:
		m.page = m.page.move(-1)
	case "J":
		m = m.scrollDetail(1)
	case "K":
		m = m.scrollDetail(-1)
	default:
		return sharedKey(m, k)
	}
	return m, nil
}
