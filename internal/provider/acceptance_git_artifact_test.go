package provider

import (
	"crypto/sha256"
	"fmt"
)

func testAccRegisterGitRepositoryRoot(repositoryRoot string) string {
	testAccRegisterArtifact(repositoryRoot)
	digest := sha256.Sum256([]byte(repositoryRoot))
	testAccRegisterArtifact(
		fmt.Sprintf(".terraform-cpanel-git-delete-%x", digest[:8]),
	)

	return repositoryRoot
}
