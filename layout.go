package herdr

// LayoutPanes returns the pane leaves of a layout tree, left to right, which
// is the order layout.apply reads its splits in.
//
// Pane ids are assigned by the server, so the only way to learn the id of a
// pane an applied layout created is to walk the tree the response carries.
// Labelling the panes in the request and matching the label here identifies
// one without depending on its position:
//
//	applied, err := client.LayoutApply(ctx, params)
//	for _, pane := range herdr.LayoutPanes(applied.Layout.Root) {
//		if Value(pane.Label) == "agent" {
//			…
//		}
//	}
//
// A nil root, and a variant a newer server adds that this package cannot
// decode, contribute no panes.
func LayoutPanes(root LayoutNode) []LayoutNodePane {
	return appendLayoutPanes(nil, root)
}

func appendLayoutPanes(panes []LayoutNodePane, node LayoutNode) []LayoutNodePane {
	switch node := node.(type) {
	case LayoutNodePane:
		return append(panes, node)
	case LayoutNodeSplit:
		return appendLayoutPanes(appendLayoutPanes(panes, node.First), node.Second)
	default:
		return panes
	}
}
