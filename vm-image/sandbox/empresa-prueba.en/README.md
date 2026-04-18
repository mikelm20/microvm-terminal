# Empresa Prueba

Shared English sandbox for learn.example.com. Same fictional
company as the Spanish sandbox; each learner works against a
single company with the same department layout. The structure
mirrors what a manager would see at work: an inbox with calendar
and emails, a people folder with contact sheets, and eight
departments with a CLAUDE.md describing what each does.

Layout:

- 00-bandeja/ - today's calendar and emails.
- 01-personas/ - sheets for relevant people.
- 01-ventas/ - sales department.
- 02-marketing/ - marketing department.
- 03-tecnologia/ - tech department.
- 04-producto/ - product department.
- 05-finanzas/ - finance department.
- 06-rrhh/ - people (HR) department.
- 07-legal/ - legal department.
- 08-estrategia/ - CEO and strategy.

From module 3 onwards, the user creates their own mi-trabajo/
folder inside Empresa Prueba with a personal CLAUDE.md and
subfolders (01-personas, 02-proyectos, 03-empresas,
04-archivo).

This folder lives at /home/learner/empresa-prueba inside each
VM. It is baked into the rootfs at build time via the Dockerfile
from vm-image/sandbox/empresa-prueba.en/ (when lang=en).
