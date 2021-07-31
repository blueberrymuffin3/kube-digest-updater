package main

import (
	"flag"
	"io/ioutil"
	"path"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	log "github.com/sirupsen/logrus"
)

var autoUpdateComment string
var packagePath string

func visit(object *yaml.RNode, p string) error {
	switch object.YNode().Kind {
	case yaml.DocumentNode:
		// Traverse the child of the document
		return visit(yaml.NewRNode(object.YNode()), p)
	case yaml.MappingNode:
		return object.VisitFields(func(node *yaml.MapNode) error {
			// Traverse each field value
			return visit(node.Value, p+"."+node.Key.YNode().Value)
		})
	case yaml.SequenceNode:
		return object.VisitElements(func(node *yaml.RNode) error {
			// Traverse each list element
			return visit(node, p)
		})
	case yaml.ScalarNode:
		return visitScalar(object, p)
	}
	return nil
}

func visitScalar(node *yaml.RNode, path string) error {
	comment := node.YNode().LineComment

	if !strings.HasSuffix(comment, autoUpdateComment) {
		return nil
	}

	// Remove any existing digest
	oldReferenceString := node.YNode().Value

	oldReference, err := name.ParseReference(strings.Split(oldReferenceString, "@")[0])
	if err != nil {
		return err
	}

	descriptor, err := remote.Get(oldReference)
	if err != nil {
		return err
	}

	newReference, err := name.ParseReference(oldReference.String() + "@" + descriptor.Digest.String())
	if err != nil {
		return err
	}

	node.YNode().SetString(newReference.String())

	log.WithFields(log.Fields{
		"path":         path,
		"oldReference": oldReferenceString,
		"newReference": newReference,
	}).Info("Updated reference")

	return nil
}

func skipFile(relPath string) bool {
	contents, err := ioutil.ReadFile(path.Join(packagePath, relPath))
	if err != nil {
		log.Error(err)
		return false
	}

	return !strings.Contains(string(contents), autoUpdateComment)
}

func main() {
	log.SetLevel(log.TraceLevel)

	flag.StringVar(&autoUpdateComment, "comment", "$update-digest$", "Lines with this comment will be updated")
	flag.StringVar(&packagePath, "directory", "", "Folder to search for yaml files")

	flag.Parse()

	if packagePath == "" {
		log.Fatal("-directory required")
	}

	filterFunc := kio.FilterFunc(func(operand []*yaml.RNode) ([]*yaml.RNode, error) {
		for i := range operand {
			resource := operand[i]

			meta, err := resource.GetMeta()
			if err != nil {
				log.Fatal(err)
			}

			err = visit(resource, "")
			if err != nil {
				log.
					WithError(err).
					WithField("name", meta.Name).
					WithField("kind", meta.Kind).
					Error("Error processing resource")
			}
		}
		return operand, nil
	})

	rw := &kio.LocalPackageReadWriter{
		PackagePath:  packagePath,
		FileSkipFunc: skipFile,
	}

	err := kio.Pipeline{
		Inputs:  []kio.Reader{rw},
		Filters: []kio.Filter{filterFunc},
		Outputs: []kio.Writer{rw},
	}.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
