# fsegit
Go言語を使用して、自作でGit開発を行っています。  
現在、`git log`、`cat-file`の実装が完了しています。  
コマンドライン化しており、`fsegit log`と`fsegit cat-file`が使用できます。
# Requirement
 
* go 1.16.6
* cobra-cli 1.6.1
 
# Installation
 
Cobra-cli<https://github.com/spf13/cobra>を使用しています。  
 
```zsh
go install github.com/spf13/cobra-cli@latest
```
 
# Usage

コマンド使用の例 
 
```zsh
git clone https://github.com/kanon1343/fsegit
cd fsegit
fsegit log
```

# テストの実行

プロジェクトのルートディレクトリで以下のコマンドを実行することで、テストを実行できます。

```zsh
go test -v ./...
```