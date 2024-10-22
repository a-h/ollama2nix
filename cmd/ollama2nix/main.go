package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Test with go run main.go -registry=registry.ollama.ai -model=mistral-nemo:7b -workers=4 -target=.

// ls ~/.ollama/models
// blobs     manifests

// ls ~/.ollama/models/blobs
// sha256:65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45
// sha256:b559938ab7a0392fc9ea9675b82280f2a15669ec3e0e0fc491c9cb0a7681cf94
// ... etc

// Fetch blobs via:
// curl https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256-65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45
// Remember to follow the redirect to the actual blob URL.

// ls ~/.ollama/models/manifests

// cat ~/.ollama/models/manifests/registry.ollama.ai/library/mistral-nemo/latest
// OR
// curl https://registry.ollama.ai/v2/library/mistral-nemo/manifests/latest
// Provide the same output
//
//	{
//	 "schemaVersion": 2,
//	 "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
//	 "config": {
//	   "digest": "sha256:65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45",
//	   "mediaType": "application/vnd.docker.container.image.v1+json",
//	   "size": 486
//	 },
//	 "layers": [
//	   {
//	     "digest": "sha256:b559938ab7a0392fc9ea9675b82280f2a15669ec3e0e0fc491c9cb0a7681cf94",
//	     "mediaType": "application/vnd.ollama.image.model",
//	     "size": 7071700672
//	   },
//	   {
//	     "digest": "sha256:f023d1ce0e55d0dcdeaf70ad81555c2a20822ed607a7abd8de3c3131360f5f0a",
//	     "mediaType": "application/vnd.ollama.image.template",
//	     "size": 688
//	   },
//	   {
//	     "digest": "sha256:43070e2d4e532684de521b885f385d0841030efa2b1a20bafb76133a5e1379c1",
//	     "mediaType": "application/vnd.ollama.image.license",
//	     "size": 11356
//	   },
//	   {
//	     "digest": "sha256:ed11eda7790d05b49395598a42b155812b17e263214292f7b87d15e14003d337",
//	     "mediaType": "application/vnd.ollama.image.params",
//	     "size": 30
//	   }
//	 ]
//	}
type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	MediaType     string  `json:"mediaType"`
	Config        Layer   `json:"config"`
	Layers        []Layer `json:"layers"`
}

type Layer struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

var flagRegistry = flag.String("registry", "registry.ollama.ai", "Registry to download models from.")
var flagModel = flag.String("model", "", "Name of the model to download, e.g. mistral-nemo, or mistral-nemo:7b")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() (err error) {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if *flagRegistry == "" {
		return fmt.Errorf("registry is required")
	}
	if *flagModel == "" {
		return fmt.Errorf("model is required")
	}
	modelVersion := strings.SplitN(*flagModel, ":", 2)
	model := modelVersion[0]
	version := "latest"
	if len(modelVersion) > 1 {
		version = modelVersion[1]
	}
	manifestURL := url.URL{
		Scheme: "https",
		Host:   *flagRegistry,
		Path:   fmt.Sprintf("/v2/library/%s/manifests/%s", url.PathEscape(model), url.PathEscape(version)),
	}
	log.Debug("Downloading manifest", slog.String("url", manifestURL.String()))

	resp, err := http.Get(manifestURL.String())
	if err != nil {
		return fmt.Errorf("failed to download manifest: %w", err)
	}
	defer resp.Body.Close()

	manifestHash := sha256.New()
	var manifest Manifest
	if err := json.NewDecoder(io.TeeReader(resp.Body, manifestHash)).Decode(&manifest); err != nil {
		return fmt.Errorf("failed to decode manifest: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("{ pkgs ? import <nixpkgs> {} }:\n")
	sb.WriteString("\n")
	sb.WriteString("let\n")
	sb.WriteString("  # List of blob files with URLs and corresponding hashes.\n")
	for i, layer := range manifest.Layers {
		blobURL := url.URL{
			Scheme: "https",
			Host:   *flagRegistry,
			Path:   fmt.Sprintf("/v2/library/mistral-nemo/blobs/%s", url.PathEscape(layer.Digest)),
		}
		sb.WriteString(fmt.Sprintf("  blob_%d = pkgs.fetchurl {\n", i))
		sb.WriteString(fmt.Sprintf("    curlOptsList = [\"-L\" \"-H\" \"Accept:application/octet-stream\"];\n"))
		sb.WriteString(fmt.Sprintf("    url = %q;\n", blobURL.String()))
		blobNixHash, err := convertOllamaHashToNixHash(layer.Digest)
		if err != nil {
			return fmt.Errorf("failed to convert blob hash: %w", err)
		}
		sb.WriteString(fmt.Sprintf("    hash = %q;\n", blobNixHash))
		sb.WriteString("  };\n")
	}
	sb.WriteString("\n")
	sb.WriteString("  # Fetch the manifest file.\n")
	sb.WriteString("  manifestFile = pkgs.fetchurl {\n")
	sb.WriteString(fmt.Sprintf("    curlOptsList = [\"-L\" \"-H\" \"Accept:application/octet-stream\"];\n"))
	sb.WriteString(fmt.Sprintf("    url = %q;\n", manifestURL.String()))
	base64Hash := base64.StdEncoding.EncodeToString(manifestHash.Sum(nil))
	sb.WriteString(fmt.Sprintf("    hash = %q;\n", "sha256-"+base64Hash))
	sb.WriteString("  };\n")
	sb.WriteString("in\n")
	sb.WriteString("  # Use symlinkJoin to create the final symlinked structure.\n")
	sb.WriteString("  pkgs.symlinkJoin {\n")
	sb.WriteString("    name = \"models\";\n")
	sb.WriteString("\n")
	sb.WriteString("    # Paths from both blobs and the manifest file.\n")
	sb.WriteString("    paths = [\n")
	for i := 0; i < len(manifest.Layers); i++ {
		sb.WriteString(fmt.Sprintf("      blob_%d\n", i))
	}
	sb.WriteString(fmt.Sprintf("      manifestFile\n"))
	sb.WriteString("    ];\n")
	sb.WriteString("\n")
	sb.WriteString("    # Add a postBuild step to arrange the structure.\n")
	sb.WriteString("    postBuild = ''\n")
	sb.WriteString("      # Move blob files to the blobs directory.\n")
	sb.WriteString("      mkdir -p $out/blobs\n")
	for i := 0; i < len(manifest.Layers); i++ {
		sb.WriteString(fmt.Sprintf("      ln -s ${blob_%d} $out/blobs/\n", i))
	}
	sb.WriteString("\n")
	sb.WriteString("      # Move manifest file to the appropriate directory.\n")
	sb.WriteString(fmt.Sprintf("      mkdir -p $out/manifests/%s/%s\n", *flagRegistry, model))
	sb.WriteString(fmt.Sprintf("      ln -s ${manifestFile} $out/manifests/%s/%s/%s\n", *flagRegistry, model, version))
	sb.WriteString("    '';\n")
	sb.WriteString("  }\n")
	fmt.Println(sb.String())
	return nil
}

func convertOllamaHashToNixHash(hexHash string) (nixHash string, err error) {
	// Remove the "sha256:" prefix
	hexHash = strings.TrimPrefix(hexHash, "sha256:")
	// Decode the hex string into bytes
	hashBytes, err := hex.DecodeString(hexHash)
	if err != nil {
		return "", err
	}
	// Encode the bytes into base64
	base64Hash := base64.StdEncoding.EncodeToString(hashBytes)
	// Return the Nix formatted hash
	return "sha256-" + base64Hash, nil
}

// Nix template.

/*
{ pkgs }:

let
  # List of blob files with URLs and corresponding hashes
  blobs = [
    {
      url = "https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256-65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45";
      sha256 = "65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45";
    }
    # Add more blobs here if needed
  ];

  # Fetch each blob file
  fetchedBlobs = map (blob: pkgs.fetchurl {
    url = blob.url;
    sha256 = blob.sha256;
  }) blobs;

  # Fetch the manifest file
  manifestFile = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/manifests/latest";
    sha256 = "65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45";
  };
in
  # Use symlinkJoin to create the final symlinked structure
  pkgs.symlinkJoin {
    name = "combined-files";

    # Paths from both blobs and the manifest file
    paths = fetchedBlobs ++ [manifestFile];

    # Add a postBuild step to arrange the structure
    postBuild = ''
      mkdir -p $out/blobs
      mkdir -p $out/manifests/registry.ollama.ai/library/mistral-nemo

      # Move blob files to the blobs directory
      for blob in ${toString (map (b: "${b}/sha256-${b.sha256}") blobs)}; do
        ln -s "$blob" "$out/blobs/"
      done

      # Move manifest file to the appropriate directory
      ln -s ${manifestFile} $out/manifests/registry.ollama.ai/library/mistral-nemo/latest
    '';
  }
*/
