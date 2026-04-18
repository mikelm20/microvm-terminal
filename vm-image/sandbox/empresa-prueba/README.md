# Empresa Prueba

Este es el sandbox compartido del curso learn.example.com.
Cada usuario aprende sobre la misma empresa ficticia. La
estructura imita lo que se encontraria un manager en su
trabajo real: una bandeja con calendario y emails, una
carpeta de personas con fichas de contacto, y ocho
departamentos con su CLAUDE.md describiendo que hacen.

Estructura:

- 00-bandeja/ - calendario y emails de hoy.
- 01-personas/ - fichas de personas relevantes.
- 01-ventas/ - departamento comercial.
- 02-marketing/ - departamento de marketing.
- 03-tecnologia/ - departamento tecnico.
- 04-producto/ - departamento de producto.
- 05-finanzas/ - departamento financiero.
- 06-rrhh/ - departamento de personas.
- 07-legal/ - departamento legal.
- 08-estrategia/ - oficina del CEO y estrategia.

A partir del modulo 3, el usuario crea su propia carpeta
mi-trabajo/ dentro de Empresa Prueba con su CLAUDE.md
personal y sus subcarpetas (01-personas, 02-proyectos,
03-empresas, 04-archivo).

Esta carpeta vive en /home/learner/empresa-prueba dentro
de cada VM. Se hornea en el rootfs en build-time desde
vm-image/sandbox/empresa-prueba/ via Dockerfile.
