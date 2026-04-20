{
  "name": "architecture",
  "version": 1,
  "layers": {
    "cmd": {
      "must_not_import": [
        "internal/cli",
        "internal/usecase",
        "internal/platform",
        "internal/domain"
      ]
    },
    "internal/cli": {
      "must_not_import": [
        "internal/platform",
        "internal/domain"
      ]
    },
    "internal/usecase": {
      "must_not_import": [
        "internal/cli",
        "internal/contract/exitcode"
      ]
    },
    "internal/platform": {
      "must_not_import": [
        "internal/cli",
        "internal/usecase",
        "internal/contract"
      ]
    },
    "internal/domain": {
      "must_not_import": [
        "internal/"
      ]
    },
    "internal/contract": {
      "must_not_import": [
        "internal/"
      ]
    }
  },
  "runtime_guards": [
    {
      "paths": [
        "internal/usecase",
        "internal/domain"
      ],
      "pattern": "os\\.(Getenv|LookupEnv|Environ)\\(",
      "message": "reads process env"
    },
    {
      "paths": [
        "internal/platform"
      ],
      "pattern": "os\\.(Stdout|Stderr)\\b",
      "message": "uses process stdio/env directly"
    },
    {
      "paths": [
        "internal/cli"
      ],
      "pattern": "^var\\s+[A-Za-z_]",
      "flags": [
        "multiline"
      ],
      "message": "contains package-level var"
    }
  ],
  "banned_directory_names": [
    "common",
    "utils",
    "wrapper",
    "wrappers",
    "helpers",
    "services",
    "builders",
    "factories",
    "managers",
    "shared",
    "facade",
    "core"
  ],
  "banned_package_names": [
    "common",
    "utils",
    "wrapper",
    "wrappers",
    "helpers",
    "services",
    "builders",
    "factories",
    "managers",
    "shared",
    "facade",
    "core"
  ],
  "banned_file_stems": [
    "common",
    "util",
    "utils",
    "wrapper",
    "wrappers",
    "helper",
    "helpers",
    "service",
    "services",
    "builder",
    "builders",
    "factory",
    "factories",
    "manager",
    "managers",
    "shared",
    "facade",
    "core"
  ],
  "allowed_internal_roots": [
    "architecture",
    "cli",
    "contract",
    "domain",
    "platform",
    "usecase"
  ],
  "allowed_cmd_roots": [
    "espops"
  ]
}
