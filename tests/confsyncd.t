Cram tests for confsyncd.
Requires my fork of cram: https://bitbucket.org/myfreeweb/cram
If you get errors (sync does not work), try increasing sleep values there.

Syncing with two instances:

  & mkdir one && cd one && touch config.json && go run $TESTDIR/../confsyncd.go -port=2048
  & mkdir two && cd two && touch config.json && go run $TESTDIR/../confsyncd.go -connect=tcp://localhost:2048
  $ sleep 2
  $ echo "from_one" > one/config.json
  $ sleep 1
  $ cat two/config.json
  from_one
  $ echo "from_two" > two/config.json
  $ sleep 1
  $ cat one/config.json
  from_two

Syncing with three instances:

  & mkdir one && cd one && touch config.json && go run $TESTDIR/../confsyncd.go -port=2048
  & mkdir two && cd two && touch config.json && go run $TESTDIR/../confsyncd.go -connect=tcp://localhost:2048
  & mkdir three && cd three && touch config.json && go run $TESTDIR/../confsyncd.go -connect=tcp://localhost:2048
  $ sleep 2
  $ echo "from_one" > one/config.json
  $ sleep 1
  $ cat two/config.json
  from_one
  $ cat three/config.json
  from_one
  $ echo "from_two" > two/config.json
  $ sleep 1
  $ cat one/config.json
  from_two
  $ cat three/config.json
  from_two
  $ echo "from_three" > three/config.json
  $ sleep 1
  $ cat one/config.json
  from_three
  $ cat two/config.json
  from_three

