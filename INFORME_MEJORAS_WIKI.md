# Informe de Auditoría y Mejoras Realizadas: Wiki del Proyecto "Trotamundos"

**Materia:** Diseño de Sistemas de Software (DSS) — Facultad de Ciencias Exactas, UNICEN  
**Proyecto:** Sistema "Trotamundos" (Trabajo Práctico Especial)  
**Marco Metodológico:** Bass, Clements & Kazman (SEI) · Cervantes & Kazman (ADD 3.0) · Michael Keeling (*Design It!*) · Sam Newman (*Building Microservices*)  
**Fecha:** Octubre 2026  

---

## 1. Resumen de la Auditoría

Se realizó una revisión integral y exhaustiva de la documentación inicial desarrollada en la Wiki del proyecto `trotamundos-dss.wiki`, contrastándola rigurosamente contra:
1. **El pliego del TPE** (`TPE - Diseño de Sistemas de Software-2.pdf`).
2. **Las Skills de referencia:**
   - `bass-software-architecture` (*Software Architecture in Practice*, 4ta Ed.).
   - `keeling-design-it` (*Design It! From Programmer to Software Architect*).
   - `newman-microservices` (*Building Microservices*, 2da Ed.).
3. **El material de la cátedra de la UNICEN** (Prof. Andrés Díaz Pace y Prof. Alfredo Teyseyre) ubicado en `FacuArchivos/IngSoftware1` (Vademécum, guías de ADD 3.0, kata Farmacy Food y checkers SEI).

### Diagnóstico General
El amigo había establecido una base conceptual válida y ordenó las páginas esenciales, pero presentaba **omisiones metodológicas críticas** que habrían resultado en observaciones severas en la corrección académica:
* **Escenarios de Calidad incompletos:** Estaban redactados como párrafos narrativos informales, omitiendo el estándar formal de **6 partes del SEI** (Fuente, Estímulo, Artefacto, Entorno, Respuesta y Medida Cuantitativa).
* **Ausencia del Árbol de Utilidad (Utility Tree):** No se había modelado la jerarquía ATAM ni la priorización bidimensional `(Importancia para el Negocio, Dificultad/Riesgo Técnico)` requerida para fundamentar los drivers `(H, H)`.
* **Proceso ADD 3.0 informal:** Las iteraciones no respetaban los **7 pasos canónicos de ADD 3.0** ni nombraban las **tácticas arquitectónicas de Bass** seleccionadas en cada paso.
* **Formato de ADRs deficiente:** No se aplicaba la plantilla formal **MADR (Markdown Architectural Decision Records)** exigida en el pliego del TPE.
* **Ausencia de patrones arquitectónicos clave de Newman y Bass:**
  - No se contemplaba el patrón **BFF (Backend for Frontend)** para diferenciar Web (latencia $< 800\text{ ms}$) de Mobile (First Contentful Paint $< 1.4\text{ s}$ y GPS).
  - No se contemplaba **CQRS** para segregar las consultas analíticas pesadas de monetización (**RF-07**) del tráfico transaccional OLTP (**CRN-01**).
  - No se contemplaba **Circuit Breaker / Outbox** para aislar fallos de la Agencia Preferida y aerolíneas externas (disponibilidad $99.988\%$).
  - La **Vista de Despliegue** era idéntica a la vista C&C y no mostraba infraestructura de nube real (Multi-AZ, balanceadores, CDN, réplicas).
  - Los contratos de API omitían el ciclo de vida del viaje (agrupación y archivado), la compartición por enlaces firmados y la especificación OpenAPI 3.0.

---

## 2. Detalle de Mejoras Aplicadas por Archivo

### 📄 `Home.md`
* Se agregó el **resumen ejecutivo** con la volumetría del pliego (15M cuentas, 2M WAU, picos semanales y disrupciones).
* Se explicitó el marco metodológico (ADD 3.0, MADR, Spec-driven, Views & Beyond).
* Se justificó la selección del módulo avanzado: **Opción B — Asistente Virtual (Chatbot)** como soporte de Nivel 1 y derivación a la agencia preferida.

### 📄 `01‐Analisis‐Inicial‐y‐Contexto.md`
* **Inclusión del actor omitido:** Se incorporó al **Equipo de Datos / BI (Data Team)** como actor externo clave para la monetización de tendencias y cancelaciones (**RF-07**).
* **Objetivos de Negocio (Business Goals):** Se formalizaron conforme a Keeling (Cap. 4) y Bass (Cap. 2): BG-01 (Retención), BG-02 (Monetización), BG-03 (Eficiencia en soporte) y BG-04 (Interoperabilidad).
* **Restricciones (CON) y Preocupaciones (CRN):** Se formalizaron `CON-01` a `CON-04` (OAuth2 obligatorio, Spec-driven, Nube Multi-Región) y `CRN-01`/`CRN-02` (Desacoplamiento OLTP/OLAP y modularidad).
* **Diagrama de Contexto:** Se rediseñó en Mermaid con flujos bidireccionales y protocolos explícitos.

### 📄 `02‐Drivers‐Arquitectonicos.md`
* **Requerimientos Funcionales (RF-01 a RF-08):** Desglose completo de escaneo de correo, whitelists, APIs de transporte, gestión manual, ciclo de vida del viaje, enlaces compartidos, analítica de monetización y chatbot.
* **Escenarios Canónicos SEI de 6 Partes:** Cada atributo de calidad (**QA-01 a QA-07**) fue reescrito en:
  1. Tabla estructurada con las 6 dimensiones formales (Fuente, Estímulo, Artefacto, Entorno, Respuesta y Medida de respuesta).
  2. Formato narrativo canónico del SEI.
  3. Métricas numéricas precisas del pliego: Web $< 800\text{ ms}$, Mobile FCP $< 1.4\text{ s}$, Ingesta $< 5\text{ min}$, Disponibilidad $99.988\%$ (máx. 5 min/mes de inactividad), nuevo transporte en $< 4\text{ días}$.
* **Árbol de Utilidad (Utility Tree):**
  - Diagrama Mermaid de 4 niveles: Utilidad $\rightarrow$ Atributos $\rightarrow$ Sub-atributos $\rightarrow$ Escenarios Hoja.
  - Priorización bidimensional ATAM `(Importancia para el Negocio, Dificultad o Riesgo Técnico)` con valores `(H, H)`, `(H, M)`, etc.
  - Identificación inequívoca de los **Drivers Críticos `(H, H)`**.

### 📄 `03‐Iteraciones‐ADD.md`
Se estructuraron las **4 iteraciones** siguiendo rigurosamente los **7 pasos formales de ADD 3.0** (Bass Cap. 20 / Cervantes / Vademécum UNICEN):
* **Iteración 1: Estructura General y Desacoplamiento Asíncrono**
  * *Drivers:* QA-03.A, QA-01.C, RF-01, RF-03.
  * *Patrones:* Microservicios, EDA, API Gateway, BFF.
  * *Tácticas de Bass:* *Manage Event Arrival (Asynchronous Queuing)*, *Increase Available Resources (Horizontal Scaling)*, *Use an Intermediary*, *Encapsulate*.
* **Iteración 2: Asistente Virtual (Chatbot) y Derivación a la Agencia Preferida**
  * *Drivers:* RF-08, QA-04.A, QA-02.A, QA-01.A.
  * *Patrones:* Autonomous Conversational Service, Circuit Breaker & Timeout, Outbox Queue.
  * *Tácticas de Bass:* *Detect Faults (Ping/Echo)*, *Prevent Faults (Circuit Breaker Open)*, *Maintain State Consistency (Idempotency Key)*.
* **Iteración 3: Rendimiento de Lectura, Alta Disponibilidad y Resiliencia en Nube**
  * *Drivers:* QA-01.A, QA-01.B, QA-02.A, QA-03.A.
  * *Patrones:* Cache-Aside (Redis Cluster), Database Read Replicas, Multi-AZ Autoscaling.
  * *Tácticas de Bass:* *Maintain Multiple Copies (Caching)*, *Active Redundancy / Spare*, *Automated Failover*, *Degrade Functionality Deliberately*.
* **Iteración 4: Seguridad, Extensibilidad y Segregación Analítica (CQRS)**
  * *Drivers:* QA-05.A, QA-05.B, QA-06.A, RF-07, CRN-01.
  * *Patrones:* JWT/OAuth2, HMAC Signed Expiring URLs, Arquitectura Hexagonal / ACL, CQRS.
  * *Tácticas de Bass:* *Authenticate Actors*, *Authorize Actors*, *Revoke Access*, *Maintain Confidentiality (AES-256 / TLS 1.3)*, *Separate Concerns / Resource Demand Arbitration*.
* **Matriz de Trazabilidad Integral:** Mapeo cruzado de Drivers $\rightarrow$ Iteración ADD $\rightarrow$ Tácticas de Bass $\rightarrow$ Patrones $\rightarrow$ ADRs $\rightarrow$ Vistas.

### 📄 `04‐ADRs.md`
Se reformularon y expandieron las decisiones a **8 ADRs exhaustivos en formato MADR formal**:
* **ADR-001:** Adopción del Estilo de Microservicios y EDA.
* **ADR-002:** Servicio Autónomo de Chatbot y Derivación Contextual con Tickets.
* **ADR-003:** Estrategia de Caché Distribuida Redis (Cache-Aside) y Redundancia Multi-AZ.
* **ADR-004:** Autenticación con OAuth 2.0 / JWT y Enlaces Temporales Firmados.
* **ADR-005:** Patrón Backend for Frontend (BFF) Diferenciado para Web y Móvil (Newman Cap. 14).
* **ADR-006:** Segregación de Consultas Analíticas y Monetización mediante CQRS (Newman Cap. 4 & Bass Cap. 9).
* **ADR-007:** Resiliencia ante APIs Externas mediante Circuit Breaker, Timeouts y Outbox (Newman Cap. 12 & Bass Cap. 4).
* **ADR-008:** Arquitectura Hexagonal y Anti-Corruption Layer (ACL) para Adaptadores de Transporte (Keeling Cap. 7 & Newman Cap. 2).

### 📄 `05‐Vistas‐Arquitectonicas.md`
* **Vista C&C Enriquecida:** Incorpora clientes Web/Móvil, CDN CloudFront, API Gateway, Web-BFF, Mobile-BFF, servicios core, clúster Redis, PostgreSQL Primaria y Réplica, Message Broker, Circuit Breakers, Adaptadores ACL y almacén analítico OLAP.
* **Clave de Notación (Legend):** Explicación explícita de semántica y protocolos (Bass Cap. 22, Keeling Cap. 10).
* **Vista de Módulos (Arquitectura Hexagonal):** Estructura de paquetes por servicio (`domain/models`, `domain/ports`, `application/services`, `infrastructure/adapters`) para cumplir el principio de *Architecturally Evident Coding Style* e *Independent Deployability*.
* **Tres Diagramas de Secuencia Dinámicos:**
  1. Consulta de itinerario con Cache-Aside (latencia $< 800\text{ ms}$).
  2. Ingesta asíncrona de correo con whitelist e invalidación de caché ($< 5\text{ min}$).
  3. Chatbot con derivación a la agencia y fallback ante apertura de Circuit Breaker.
* **Vista de Despliegue en la Nube (AWS Multi-AZ Allocation View):** Modelado real de infraestructura: Route 53, CloudFront, WAF, Application Load Balancers, dos Zonas de Disponibilidad (AZ-A y AZ-B), contenedores ECS Fargate, clúster ElastiCache Redis, Amazon RDS Multi-AZ y Data Lake S3/Athena.
* **Modelo de Datos Entidad-Relación (ER):** Entidades completas para Usuarios, Viajes, Reservas polimórficas (vuelos, hoteles, transporte), Enlaces compartidos firmados, Sesiones de chat y Tickets de derivación.

### 📄 `06‐Especificaciones‐API.md`
* **Contratos completos del ciclo de vida:** Creación y agrupación de viajes (`POST /api/v1/viajes`), archivado automático (`PUT /api/v1/viajes/{viajeId}/archivar`), y gestión manual (`POST /api/v1/reservas`).
* **Contratos de seguridad y compartición:** Generación de enlaces firmados (`POST /api/v1/viajes/{viajeId}/compartir`) y endpoint público validado (`GET /api/v1/compartido/{token}`).
* **Configuración de correo:** Whitelist de remitentes seguros (`POST /api/v1/usuarios/{usuarioId}/filtros-correo`).
* **Chatbot y Derivación:** Interacción Nivel 1 y derivación con transfer de contexto (`POST /api/v1/chatbot/consulta` y `POST /api/v1/agencia/derivacion`).
* **Endpoint de Analítica:** Consultas pesadas de monetización (`GET /api/v1/analitica/tendencias`).
* **Idempotencia y Errores:** Manejo de cabecera `Idempotency-Key` y estándar RFC 7807.
* **Extracto OpenAPI 3.0 (YAML):** Especificación formal para desarrollo guiado por contratos (*Spec-driven Development*).

---

## 3. Estado de Versiones y Próximos Pasos

Todos los cambios fueron consolidados y comiteados en el repositorio local de la wiki:
* Commit: `76e8224 docs: alinear wiki con metodologia ADD 3.0, SEI 6 partes, MADR y patrones Newman/Bass`

### Pasos Sugeridos para la Implementación del Prototipo:
1. Crear el repositorio de código fuente en `trotamundos-dss/` siguiendo la estructura de la Vista de Módulos (Arquitectura Hexagonal).
2. Desarrollar un prototipo ejecutable liviano (ej. Node.js/TypeScript o Python/FastAPI) que implemente:
   - Los endpoints del Web/Mobile BFF para consulta con caché Redis en memoria.
   - La ingesta asíncrona de correo mediante una cola simulada.
   - El endpoint del Chatbot con el clasificador de intenciones y el mock de la Agencia de Viajes con Circuit Breaker.
