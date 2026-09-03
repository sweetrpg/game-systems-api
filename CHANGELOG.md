
## 0.4.0 - 2026-09-03

### <!-- 0 -->🚀 Features
- Adopt platform audit-fields convention on EntityMeta
- Carry the meta audit block in GET/POST/PATCH /systems responses


### <!-- 1 -->🐛 Bug Fixes
- Key version writes on the canonical meta _id, not the raw id arg


### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.3.0



## 0.3.0 - 2026-09-02

### <!-- 0 -->🚀 Features
- Publish NATS change events on committed system mutations


### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.2.4



## 0.2.4 - 2026-09-02

### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.2.3
- Update to standard labels



## 0.2.3 - 2026-08-28

### <!-- 2 -->🚜 Refactor
- Rename gamesystems references to game-systems


### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.2.2



## 0.2.2 - 2026-08-25

### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.2.1
- Tighten GOGC to 90 on gamesystems-api deployment



## 0.2.1 - 2026-08-25

### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.2.0
- Set GOMEMLIMIT and GOGC on gamesystems-api deployment



## 0.2.0 - 2026-08-23

### <!-- 0 -->🚀 Features
- Add OpenFeature annotations to deployment
- Add unique system_id slug to game system meta records


### <!-- 7 -->⚙️ Miscellaneous Tasks
- Merge master into develop after v0.1.0



## 0.1.0 - 2026-08-20

### <!-- 0 -->🚀 Features
- Rebuild as a Go service (Gin + api-core.go)
- Add base + dev/local overlay manifests


### <!-- 7 -->⚙️ Miscellaneous Tasks
- Start work on game-systems-service (#53)
- Scaffold repo baseline (CI, dependabot, community docs)


## Unreleased

### Added
- Repo scaffolding: CI/PR/release workflows, dependabot, branch protection, community docs
