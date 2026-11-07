-- Migration inicial do Nelisa. Até a ida para a AWS esta é a única
-- migration e pode ser editada: o banco é recriado do zero (ver
-- docs/plano.md). Depois do congelamento, só migrations novas.

-- +goose Up
-- Um schema por módulo (docs/arquitetura.md). Cada módulo acrescenta aqui
-- as próprias tabelas.
CREATE SCHEMA IF NOT EXISTS team;
CREATE SCHEMA IF NOT EXISTS audit;

-- +goose Down
DROP SCHEMA IF EXISTS audit CASCADE;
DROP SCHEMA IF EXISTS team CASCADE;
