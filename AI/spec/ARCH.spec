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
  ]
}
