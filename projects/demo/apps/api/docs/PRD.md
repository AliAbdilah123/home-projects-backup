# PRD: Multi-Agent Task Orchestration

## Problem
Tim engineering saat ini mengandalkan koordinasi manual saat eksekusi engineering. Hasilnya: Task menumpuk, tidak ada who-does-what yang jelas, dan quality gate berantakan. Produktifitas engineer turun.

## Solution Overview
Build internal toolchain “**AIOPS LITE**” untuk:
1) Menerima ide/work-request
2) Menghasilkan PRD + API contract otomatis
3) Melakukan ordinasi ke *multi-agent worker* sesuai complexity
4) Minimalisir friction di fase preview sebelum integrasi lengkap

## Goals
- Operator mampu melihat PRD dan API contract yang valid
- Operator mampu mengirimkan *workboard request* dari dashboard ini
- Komponen dasar layanan sudah dijalankan dan diuji minimal
- Staging URL sudah hidup untuk validasi cepat

## Non-Goals
- Belum full CI/CD
- Belum production-grade monitoring
- Belum self-paced autonomous loop tanpa approval

## User Stories
- US-1: Dapat generate PRD + API contract dari ide kerja
- US-2: Dapat submit task ke board dari UI
- US-3: Dapat select model/tier untuk setiap task

## Technical Approach
- Frontend: TypeScript + React + Vite
- Backend: Go HTTP server
- Database: SQLite
- Exposed via nginx at `/projects/demo`

## Success Criteria
- PRD + contract dapat diperiksa tanpa restart server
- Submit task berhasil dari dashboard ke kanban
- Staging URL dapat diakses tanpa error 502
