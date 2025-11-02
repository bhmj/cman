# Container Manager service (CMan)

## What is it?

CMan is a backend service running on Linux platform providing API for compiling and running various programming language sources.

Inside it is a config-based container manager with both system-wide and per-request resource limiting, automatic container cleanup, reusable runners and real time response streaming.

## Getting started

  1. clone the repo
  2. `make setup`
  3. `cd docker-assets && ./build-images.sh run && cd ..`
  3. `make dev-up`
  4. run the test query:

  ```bash
    curl --request POST \
      --url http://127.0.0.1:8260/api/run/ \
      --header 'Api-Token: test-token' \
      --header 'Content-Type: application/json' \
      --data '{ "files": { "main.go": "package main\r\n\r\nimport \"fmt\"\r\n\r\nfunc main() {\r\n\tfmt.Println(\"Hello!\")\r\n}" }, "main": "main.go", "lang": "go", "version": "1.23", "runtime": 50 }'
  ```

## Changelog

**0.3.0** (2025-11-02) -- Public release.

## Contributing

1. Fork it!
2. Create your feature branch: `git checkout -b my-new-feature`
3. Commit your changes: `git commit -am 'Add some feature'`
4. Push to the branch: `git push origin my-new-feature`
5. Submit a pull request :)

## Licence

[MIT](http://opensource.org/licenses/MIT)

## Author

Michael Gurov aka BHMJ
