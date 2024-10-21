{ pkgs ? import <nixpkgs> {} }:

let
  # List of blob files with URLs and corresponding hashes.
  blob_0 = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256:b559938ab7a0392fc9ea9675b82280f2a15669ec3e0e0fc491c9cb0a7681cf94";
    hash = "sha256:b559938ab7a0392fc9ea9675b82280f2a15669ec3e0e0fc491c9cb0a7681cf94";
  };
  blob_1 = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256:f023d1ce0e55d0dcdeaf70ad81555c2a20822ed607a7abd8de3c3131360f5f0a";
    hash = "sha256:f023d1ce0e55d0dcdeaf70ad81555c2a20822ed607a7abd8de3c3131360f5f0a";
  };
  blob_2 = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256:43070e2d4e532684de521b885f385d0841030efa2b1a20bafb76133a5e1379c1";
    hash = "sha256:43070e2d4e532684de521b885f385d0841030efa2b1a20bafb76133a5e1379c1";
  };
  blob_3 = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/blobs/sha256:ed11eda7790d05b49395598a42b155812b17e263214292f7b87d15e14003d337";
    hash = "sha256:ed11eda7790d05b49395598a42b155812b17e263214292f7b87d15e14003d337";
  };

  # Fetch the manifest file.
  manifestFile = pkgs.fetchurl {
    url = "https://registry.ollama.ai/v2/library/mistral-nemo/manifests/latest";
    hash = "sha256:65d37de20e5951c7434ad4230c51a4d5be99b8cb7407d2135074d82c40b44b45";
  };
in
  # Use symlinkJoin to create the final symlinked structure.
  pkgs.symlinkJoin {
    name = "models";

    # Paths from both blobs and the manifest file.
    paths = [
      blob_0
      blob_1
      blob_2
      blob_3
      manifestFile
    ];

    # Add a postBuild step to arrange the structure.
    postBuild = ''
      # Move blob files to the blobs directory.
      mkdir -p $out/blobs
      ln -s ${blob_0} $out/blobs/
      ln -s ${blob_1} $out/blobs/
      ln -s ${blob_2} $out/blobs/
      ln -s ${blob_3} $out/blobs/

      # Move manifest file to the appropriate directory.
      mkdir -p $out/manifests/registry.ollama.ai/mistral-nemo
      ln -s ${manifestFile} $out/manifests/registry.ollama.ai/mistral-nemo/latest
    '';
  }

