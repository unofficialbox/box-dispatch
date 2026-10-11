package webapi

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

var errStaleValidation = errors.New(staleValidationMessage)

const validationContextVersion = 1
const staleValidationMessage = "The package or selected connection changed or could not be verified. Validate again before starting deployment."
const activeContextMessage = "Validation or deployment is active. Wait for its result before changing the package or selected connection."

type validationContextReader func(config.SolutionPlan, config.ConnectionSettings) (string, error)

// Package bytes, effective configuration and destination are bound together.
// This detects drift; it is not an immutable snapshot against external writers.
func fingerprintValidationContext(plan config.SolutionPlan, settings config.ConnectionSettings) (string, error) {
	for _, provider := range plan.Components {
		switch provider {
		case "box":
			target, ok := settings.SelectedBoxConnection()
			if !ok || (target.SubjectID == "" && (target.Enterprise == "" || (target.Account == "" && target.Identity == ""))) {
				return "", fmt.Errorf("selected Box identity is unavailable")
			}
		case "salesforce":
			target, ok := settings.SelectedSalesforceOrg()
			if !ok || target.OrgID == "" || target.InstanceURL == "" {
				return "", fmt.Errorf("selected Salesforce identity is unavailable")
			}
		}
	}
	root, err := filepath.Abs(plan.PackagePath)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("package directory is unavailable")
	}
	hash := sha256.New()
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	hash.Write(data)
	hash.Write([]byte(config.DeploymentTargetFingerprint(settings)))
	// These nonsecret overrides determine the effective Box destination; the
	// remaining runtime target fields are in the hashed package configuration.
	targetEnvironment, _ := json.Marshal(struct {
		Parent, Workspace string
		AllowRoot         bool
	}{
		strings.TrimSpace(os.Getenv("BOX_PARENT_FOLDER_ID")), strings.TrimSpace(os.Getenv("BOX_WORKSPACE_NAME")),
		strings.EqualFold(strings.TrimSpace(os.Getenv("BOX_ALLOW_ROOT_FOLDER")), "true"),
	})
	hash.Write(targetEnvironment)
	// Read the reference without solution.Load's deployment-config creation.
	reference := ".dispatch/deployment.bcl"
	if document, err := bcl.LoadBCL(filepath.Join(root, "dispatch.bcl")); err == nil {
		data, _ := json.Marshal(document.Metadata["manifest"])
		var manifest struct {
			DeploymentConfig string `json:"deployment_config"`
		}
		if json.Unmarshal(data, &manifest) == nil && manifest.DeploymentConfig != "" {
			reference = filepath.ToSlash(filepath.Clean(manifest.DeploymentConfig))
		}
	}
	var bytesRead int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		containsReference := name == reference || strings.HasPrefix(reference, name+"/")
		if entry.IsDir() && !containsReference && (entry.Name() == ".git" || entry.Name() == "node_modules" || name == "dispatch/audit") {
			return filepath.SkipDir
		}
		// Only known operator state is excluded. Deployment settings, including
		// custom manifest references inside .dispatch, remain meaningful inputs.
		if name != reference && (name == ".dispatch/connection-settings.bcl" || name == ".dispatch/solution-plan.bcl" || name == ".dispatch/deployment-defaults.bcl") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package links are not supported for validation context")
		}
		if entry.IsDir() {
			return nil
		}
		before, err := entry.Info()
		if err != nil {
			return err
		}
		if !before.Mode().IsRegular() {
			return fmt.Errorf("package contains an unsupported file")
		}
		bytesRead += before.Size()
		if bytesRead > 1<<30 {
			return fmt.Errorf("package exceeds validation context limit")
		}
		binary.Write(hash, binary.LittleEndian, uint64(len(name)))
		hash.Write([]byte(name))
		binary.Write(hash, binary.LittleEndian, uint64(before.Size()))
		binary.Write(hash, binary.LittleEndian, uint32(before.Mode().Perm()))
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		count, readErr := io.Copy(hash, file)
		after, statErr := file.Stat()
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if statErr != nil {
			return statErr
		}
		if closeErr != nil {
			return closeErr
		}
		if count != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("package changed during fingerprinting")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
