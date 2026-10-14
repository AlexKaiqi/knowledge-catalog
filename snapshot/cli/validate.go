package cli

func (e *executor) Validate(path string, flags map[string]any) error {
	if err := e.ValidateFlags(path, flags); err != nil {
		return err
	}
	if !hasCommand(path) {
		return nil // A composing host may validate its own other command families.
	}
	if flags["help"] == true {
		return nil
	}
	switch path {
	case "dataset define":
		if _, err := requireRemoteFlag(flags, "dataset"); err != nil {
			return err
		}
		if _, err := remoteIntFlag(flags, "revision"); err != nil {
			return err
		}
		_, err := remoteDatasetInput(flags)
		return err
	case "dataset retire":
		_, err := requireRemoteFlag(flags, "dataset")
		return err
	case "attach", "detach":
		_, err := requireRemoteFlag(flags, "repo")
		return err
	case "dataset show":
		_, _, err := datasetShowInput(flags)
		return err
	case "files list", "files read":
		_, revision, _, err := fileCommandInput(path, flags)
		if err != nil {
			return err
		}
		if path == "files list" {
			_, _, err = fileListControls(flags, revision)
		} else {
			_, _, _, err = fileReadControls(flags)
		}
		return err
	case "grant update":
		_, _, err := grantUpdateRequestFromFlags(flags)
		return err
	}
	return nil
}
