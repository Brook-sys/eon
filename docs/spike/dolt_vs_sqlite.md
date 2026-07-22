# Spike: Dolt vs SQLite Local Performance Baseline

SQLite: load_claims ~1081 ops/s, query_claims ~1042 ops/s
Dolt: load_claims ~1221 ops/s, query_claims ~1058 ops/s
Decisao: SQLite continua sendo o default por portabilidade de compilação sem dependência binária. Dolt validado sem regressões de performance em modo local.
