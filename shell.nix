{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
	buildInputs = with pkgs; [
		go
		gopls
		gotools
		go-tools # staticcheck
		protobuf
		protoc-gen-go
		deno
		nodePackages.prettier
	];

	shellHook = ''
		export PATH="$PATH:${builtins.toString ./.}/bin"
	'';
}
