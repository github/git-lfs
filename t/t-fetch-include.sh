#!/usr/bin/env bash

. "$(dirname "$0")/testlib.sh"

reponame="$(basename "$0" ".sh")"
contents="big file"
contents_oid=$(calc_oid "$contents")

begin_test "fetch: setup for include test"
(
  set -e

  setup_remote_repo "$reponame"
  clone_repo "$reponame" repo

  git lfs track "*.big"
  mkdir -p big/a
  mkdir -p big/b

  printf "%s" "$contents" > big/a/a1.big
  printf "%s" "$contents" > big/b/b1.big

  contents2="big file 2"
  printf "%s" "$contents2" > big/big1.big
  printf "%s" "$contents2" > big/big2.big
  printf "%s" "$contents2" > big/big3.big

  git add .gitattributes big
  git commit -m "commit" | tee commit.log
  grep "6 files changed" commit.log
  grep "create mode 100644 .gitattributes" commit.log
  grep "create mode 100644 big/a/a1.big" commit.log
  grep "create mode 100644 big/b/b1.big" commit.log
  grep "create mode 100644 big/big1.big" commit.log
  grep "create mode 100644 big/big2.big" commit.log
  grep "create mode 100644 big/big3.big" commit.log

  git push origin main | tee push.log
  grep "Uploading LFS objects: 100% (2/2), 18 B" push.log

  assert_server_object "$reponame" "$contents_oid"
)
end_test

begin_test "fetch: include first matching file"
(
  set -e

  mkdir clone-1
  cd clone-1
  git init
  git lfs install --local --skip-smudge
  git remote add origin $GITSERVER/$reponame
  git pull origin main

  refute_local_object "$contents_oid"

  git lfs ls-files

  git lfs fetch --include=big/a/**

  assert_local_object "$contents_oid" "8"
)
end_test

begin_test "fetch: include second matching file"
(
  set -e

  mkdir clone-2
  cd clone-2
  git init
  git lfs install --local --skip-smudge
  git remote add origin $GITSERVER/$reponame
  git pull origin main

  refute_local_object "$contents_oid"

  git lfs ls-files

  git lfs fetch --include=big/b/**

  assert_local_object "$contents_oid" "8"
)
end_test
