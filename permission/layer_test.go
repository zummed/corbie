package permission

func allow(entries ...string) Layer { return Layer{Name: "the layer", Allow: entries} }
