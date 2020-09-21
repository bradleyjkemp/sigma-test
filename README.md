# sigma-test
`sigma-test` is a test case runner for [Sigma](https://github.com/Neo23x0/sigma) rules.
It lets you specify example events alongside your detection rules and assert that your rule matches what you expect.

Install:
* via Homebrew: `brew install bradleyjkemp/formulae/sigma-test`
* via GitHub releases: download the latest binary [here](https://github.com/bradleyjkemp/sigma-test/releases/latest).
* From source: `go get github.com/bradleyjkemp/sigma-test`

⚠️ `sigma-test` **evaluates rules using [sigma-go](https://github.com/bradleyjkemp/sigma-go) which is still under development. Some syntax may not be supported yet.**

## Usage

Let's take a look at a simple (correct) example:
```yaml
title: Example of using sigma-test
description: Demos a passing sigma-test rule

detection:
  ssh:
    dst_port: 22
  permitted_user:
    user:
      - alice
      - bob
  condition: ssh and not permitted_user
# A standard(ish) Sigma rule 👆 

# How to add your test cases 👇
testcases:
  match:
    - dst_port: 22
      user: charlie

  dont-match:
    # Shouldn't match non ssh traffic
    - dst_port: 443
      user: charlie

    # Shouldn't match authorized users
    - dst_port: 22
      user: alice
``` 
Running `sigma-test` outputs that, as expected, the tests passed:
```bash
> sigma-test ./rules

rules/example.yaml          PASS
```

If a test fails, `sigma-test` tells you why:
```bash
> sigma-test ./rules/broken.yaml

rule/broken.yaml     FAIL    
                     map[dst_port:22] should have matched

exit status 1
```