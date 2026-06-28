# Arquitectura — App de finanzas personales
**Stack:** Go · React · Supabase (Postgres) · Vercel · Railway  
**Autor:** Erne · **Fecha:** Junio 2026  
**Propósito de este doc:** entender el *por qué* de cada decisión, no solo el qué.

---

## Índice

1. [Visión general](#1-visión-general)
2. [Por qué este stack y no otro](#2-por-qué-este-stack-y-no-otro)
3. [Por qué Postgres y no MongoDB](#3-por-qué-postgres-y-no-mongodb)
4. [Arquitectura de capas en Go](#4-arquitectura-de-capas-en-go)
5. [Autenticación — cómo funciona el flujo JWT](#5-autenticación--cómo-funciona-el-flujo-jwt)
6. [Supabase como proveedor de auth y base de datos](#6-supabase-como-proveedor-de-auth-y-base-de-datos)
7. [Schema de la base de datos — decisiones tabla por tabla](#7-schema-de-la-base-de-datos--decisiones-tabla-por-tabla)
8. [Row Level Security (RLS) — por qué es crítico](#8-row-level-security-rls--por-qué-es-crítico)
9. [Import CSV — diseño del parser](#9-import-csv--diseño-del-parser)
10. [Estructura de carpetas del proyecto Go](#10-estructura-de-carpetas-del-proyecto-go)
11. [Deploy — Vercel + Railway](#11-deploy--vercel--railway)
12. [Lo que se descartó y por qué](#12-lo-que-se-descartó-y-por-qué)
13. [Roadmap técnico por fases](#13-roadmap-técnico-por-fases)

---

## 1. Visión general

Esta app permite a múltiples usuarios registrar, categorizar y analizar sus gastos personales. Soporta carga manual y carga masiva via CSV exportado desde Apple Wallet o bancos argentinos (BBVA, Ciudad).

### Diagrama de flujo de una request típica

```
[Browser React]
     │
     │  1. Usuario hace signIn con Supabase SDK (directo al servicio de auth)
     │  2. Supabase devuelve un JWT
     │
     ▼
[API Go en Railway]
     │  3. React manda el JWT en el header: Authorization: Bearer <token>
     │  4. Go valida el JWT con la clave pública de Supabase (sin red, localmente)
     │  5. Go extrae el user_id del JWT
     │  6. Go ejecuta la query en Postgres, filtrando SIEMPRE por user_id
     │
     ▼
[Postgres en Supabase]
     │  7. Devuelve datos
     ▼
[API Go] → [React] → [Pantalla]
```

**La decisión central:** React nunca habla directamente con la DB. Todo pasa por Go. Supabase es solo infraestructura (auth + postgres), no el backend de la aplicación.

---

## 2. Por qué este stack y no otro

### Por qué Go para el backend

**La razón honesta:** querés practicar Go para entrevistas en Mercado Libre, que lo usa en producción extensamente. Pero Go también es objetivamente una buena elección técnica para este caso.

**Razones técnicas:**
- **Compilado y tipado estáticamente.** Los errores aparecen en tiempo de compilación, no en runtime cuando un usuario ya lo rompió. Esto importa mucho en lógica financiera donde un tipo incorrecto puede significar un monto mal calculado.
- **Manejo de errores explícito.** Go te obliga a manejar cada error: `result, err := db.Query(...)`. No hay excepciones silenciosas. En un contexto financiero, no querés que un error se propague sin ser manejado.
- **`net/http` estándar es suficiente.** Para una API REST no necesitás un framework pesado. Go incluye un servidor HTTP en su librería estándar que es production-ready.
- **Concurrencia nativa.** Goroutines y channels permiten procesar miles de filas de CSV sin bloquear el servidor. Relevante para el import.
- **Un solo binario como output.** El build produce un ejecutable único. Deploy = copiar un archivo. No hay dependencias de runtime como Node o Python.

**¿Por qué no Node/Express?** También sería válido. La diferencia real: Go te enseña a pensar en tipos, manejo de errores y concurrencia de manera más explícita. Para aprender, es más formativo. Para producción, ambos son aptos.

**¿Por qué no NestJS?** Ya usás NestJS en GestColar. Este proyecto es una oportunidad de salir de esa zona de confort.

### Por qué React + Vite

React porque ya lo conocés (Angular es similar en conceptos). Vite porque es el bundler moderno — más rápido que Webpack/CRA, sin configuración innecesaria.

**¿Por qué no Angular?** Podría ser Angular. La elección de React es pragmática: el ecosistema de librerías de UI (shadcn, recharts) es más maduro en React, y la mayoría de ofertas laborales en Argentina piden React.

### Por qué Vercel para el frontend

Vercel es el deploy más simple que existe para React/Vite: conectás el repo de GitHub, pusheás, y en 30 segundos está deployado con HTTPS, CDN global y preview automático por PR. El plan gratuito es suficiente para un side project. No tiene sentido gastar tiempo en configurar Nginx o un VPS para servir archivos estáticos.

### Por qué Railway para Go

Railway es la opción más simple para deployar un binario Go con variables de entorno. Detecta automáticamente que es un proyecto Go, lo compila, y lo levanta. Alternativa válida: Fly.io (más control, mismo nivel de simplicidad). Se descartó Heroku porque su plan gratuito desapareció en 2022.

---

## 3. Por qué Postgres y no MongoDB

Esta es una de las preguntas más importantes de arquitectura. La respuesta no es "relacional es siempre mejor" — es "estos datos en particular tienen estructura relacional".

### La forma de los datos financieros

Una transacción en esta app tiene relaciones fijas y predecibles:
- Pertenece a **un** usuario (no puede no tener usuario)
- Tiene **una** categoría (o ninguna, pero no varias)
- Puede pertenecer a **un** batch de import (o ninguno)

Esto es exactamente para lo que fue diseñada una base de datos relacional. Las FK (foreign keys) garantizan integridad referencial: no podés tener una transacción apuntando a una categoría que no existe.

### El problema concreto con MongoDB acá

Supongamos que guardás así en Mongo:

```json
{
  "amount": 1500,
  "description": "Mercado Libre",
  "category": {
    "name": "compras",
    "color": "#4A90E2",
    "icon": "shopping-cart"
  },
  "userId": "abc123"
}
```

**Problema 1 — Desnormalización:** si el usuario renombra la categoría "compras" a "online shopping", tenés que actualizar TODOS los documentos que la referencian. En Postgres, actualizás una sola fila en la tabla `categories` y todas las transacciones la ven automáticamente via FK.

**Problema 2 — Aggregations:** para mostrar "gastos del mes por categoría" en Mongo necesitás:
```js
db.transactions.aggregate([
  { $match: { userId: "abc123", date: { $gte: startOfMonth } } },
  { $group: { _id: "$category.name", total: { $sum: "$amount" } } }
])
```

En SQL:
```sql
SELECT c.name, SUM(t.amount)
FROM transactions t
JOIN categories c ON t.category_id = c.id
WHERE t.user_id = 'abc123'
AND date_trunc('month', t.date) = date_trunc('month', NOW())
GROUP BY c.name;
```

El SQL es más legible, más mantenible, y el query planner de Postgres lo optimiza automáticamente con índices.

**Problema 3 — Transacciones ACID:** si el import CSV falla a mitad, en Postgres podés hacer rollback y nada quedó guardado. En MongoDB (a menos que uses transacciones multi-documento que son lentas), podés quedar con datos parciales.

### ¿Cuándo MongoDB sería la elección correcta?

Si los datos fueran inherentemente no estructurados. Ejemplo: si cada banco te manda un JSON con campos completamente distintos y necesitás guardarlo "crudo" para procesarlo después. Pero incluso ahí, Postgres tiene el tipo `JSONB` que te permite guardar JSON arbitrario Y consultarlo con índices. Tenés lo mejor de ambos mundos.

### Una nota sobre JSONB en Postgres

En la tabla `transactions` hay un campo `raw_import_data JSONB` para guardar la fila original del CSV sin procesar. Esto demuestra que Postgres no es rígido — te permite un campo flexible cuando lo necesitás, sin sacrificar la integridad del resto.

---

## 4. Arquitectura de capas en Go

El proyecto Go se divide en capas con responsabilidades claras. Este patrón se llama **Repository Pattern** y es estándar en la industria.

```
handlers/ → services/ → repository/ → DB
```

### Por qué separar en capas y no todo junto

**El principio:** cada función debería tener una sola razón para cambiar.

Si metés todo en un handler:
```go
// ❌ MAL — el handler hace demasiado
func CreateTransaction(w http.ResponseWriter, r *http.Request) {
    // parsea el body JSON
    // valida que amount sea positivo
    // verifica que la categoría exista
    // deduplica contra transacciones recientes
    // inserta en la DB
    // envía respuesta
}
```

Si el día de mañana cambiás la DB de Postgres a otra cosa, tenés que reescribir el handler entero. Si querés testear la lógica de deduplicación, tenés que levantar un servidor HTTP entero.

**Con capas:**

```go
// ✅ BIEN — cada capa tiene una responsabilidad

// handler: solo HTTP (parsear request, escribir response)
func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateTransactionRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    result, err := h.service.CreateTransaction(r.Context(), req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    json.NewEncoder(w).Encode(result)
}

// service: solo lógica de negocio (validaciones, reglas, deduplicación)
func (s *TransactionService) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (*Transaction, error) {
    if req.Amount <= 0 {
        return nil, errors.New("el monto debe ser positivo")
    }
    if isDuplicate, _ := s.repo.CheckDuplicate(ctx, req); isDuplicate {
        return nil, errors.New("transacción duplicada")
    }
    return s.repo.Insert(ctx, req)
}

// repository: solo acceso a datos (queries SQL, nada más)
func (r *TransactionRepository) Insert(ctx context.Context, t CreateTransactionRequest) (*Transaction, error) {
    row := r.db.QueryRow(ctx, 
        "INSERT INTO transactions (user_id, amount, description, category_id, date) VALUES ($1, $2, $3, $4, $5) RETURNING *",
        t.UserID, t.Amount, t.Description, t.CategoryID, t.Date,
    )
    // ...
}
```

**Ventajas concretas:**
- Podés testear el service sin DB (usando un mock del repository)
- Podés cambiar el ORM/driver sin tocar la lógica de negocio
- Podés leer el código y entender dónde está cada cosa sin buscar

### La capa middleware

El middleware es código que se ejecuta **antes** de que el request llegue al handler. Se usa para lógica transversal a todos los endpoints.

```go
// Cada request pasa por esto antes de llegar al handler
func ValidateJWT(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        // valida el JWT
        // extrae el user_id
        // lo pone en el context del request
        next.ServeHTTP(w, r) // pasa al siguiente handler
    })
}
```

**¿Por qué context y no una variable global?**
Porque en Go cada request se maneja en su propia goroutine (hilo ligero). Una variable global sería compartida entre todos los requests concurrentes. El `context.Context` es por-request — cada request tiene el suyo.

---

## 5. Autenticación — cómo funciona el flujo JWT

### Qué es un JWT

Un JWT (JSON Web Token) es un string en tres partes separadas por puntos:

```
eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoiYWJjMTIzIiwiZXhwIjoxNzE5NTk5NjAwfQ.xK3...
    ^                       ^                                                   ^
  Header (base64)      Payload (base64)                               Firma (HMAC/RSA)
```

El payload decodificado contiene:
```json
{
  "sub": "abc123-user-id",
  "email": "erne@gmail.com",
  "exp": 1719599600,
  "role": "authenticated"
}
```

**Lo importante:** el JWT está *firmado* por Supabase con su clave privada. Go puede verificar que la firma es válida usando solo la clave *pública* de Supabase — sin hacer ninguna llamada de red. Esto lo hace muy rápido.

### El flujo completo paso a paso

```
1. Usuario ingresa email + password en React
2. React llama: supabase.auth.signInWithPassword({ email, password })
3. Supabase valida las credenciales y devuelve un JWT
4. React guarda el JWT en memoria (o localStorage)

5. React necesita cargar transacciones:
   fetch('https://api.tuapp.railway.app/transactions', {
     headers: { 'Authorization': `Bearer ${jwt}` }
   })

6. Go recibe el request
7. Go extrae el token del header
8. Go verifica la firma con la clave pública de Supabase (local, sin red)
9. Go decodifica el payload y extrae: sub = "abc123-user-id"
10. Go pone el user_id en el context del request
11. El handler usa ctx.Value("user_id") para filtrar datos

12. Go hace query: SELECT * FROM transactions WHERE user_id = 'abc123-user-id'
13. Go responde con los datos
14. React los muestra
```

### Por qué validar en Go y no confiar en el frontend

El frontend podría mandar cualquier user_id en un request malicioso:
```
GET /transactions?user_id=otro-usuario-id
```

Si Go confiara en el parámetro, cualquiera vería los datos de cualquiera. Al extraer el user_id del JWT (que está firmado por Supabase), garantizamos que el user_id es auténtico — nadie puede falsificarlo sin la clave privada de Supabase.

### Cuándo expira el JWT y cómo se renueva

Supabase emite JWTs con expiración corta (1 hora por defecto) y un refresh token de larga duración. El SDK de Supabase en el frontend renueva automáticamente el JWT antes de que expire. Tu API Go no necesita saber nada de esto.

---

## 6. Supabase como proveedor de auth y base de datos

### Qué usa Supabase en este proyecto y qué no

| Feature de Supabase | ¿Se usa? | Por qué |
|---|---|---|
| PostgreSQL | ✅ Sí | Es la DB del proyecto |
| Auth (JWT) | ✅ Sí | Login/registro de usuarios |
| SDK de JS (cliente) | ✅ Parcial | Solo para signIn/signUp en React |
| PostgREST (API automática) | ❌ No | Go reemplaza esto completamente |
| Realtime | ❌ No | No es necesario por ahora |
| Storage | ❌ No | No hay archivos que persistir |
| Edge Functions | ❌ No | Go las reemplaza |
| RLS (Row Level Security) | ⚠️ Opcional | Go ya filtra por user_id, pero RLS es una segunda línea de defensa |

**La decisión clave:** Supabase tiene una API REST automática (PostgREST) que permite al frontend hacer queries directamente a la DB. **No la usamos.** Todo pasa por Go. Esto puede parecer trabajo extra, pero:

1. Go te permite agregar lógica de negocio entre el request y la DB
2. Go puede combinar múltiples queries antes de responder
3. Si algún día cambiás de Supabase a otra DB, React no necesita cambiar nada — solo Go

### La connection string de Supabase

Go se conecta a Supabase como si fuera cualquier Postgres:
```
postgresql://postgres.[project-ref]:[password]@aws-0-us-east-1.pooler.supabase.com:6543/postgres
```

Esta es la connection string del connection pooler de Supabase (PgBouncer). **No uses la directa** para una app con múltiples usuarios concurrentes — el pooler maneja el límite de conexiones.

---

## 7. Schema de la base de datos — decisiones tabla por tabla

### `profiles`

```sql
CREATE TABLE profiles (
    id          UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    display_name TEXT,
    currency    TEXT NOT NULL DEFAULT 'ARS',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Por qué `id` es FK a `auth.users`:** Supabase maneja los usuarios en un schema interno llamado `auth`. Al referenciar `auth.users`, garantizamos que cada profile corresponde a un usuario real. El `ON DELETE CASCADE` hace que si un usuario se borra, su profile también.

**Por qué un trigger para crear el profile automáticamente:**
```sql
CREATE FUNCTION public.handle_new_user()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO public.profiles (id) VALUES (NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

CREATE TRIGGER on_auth_user_created
    AFTER INSERT ON auth.users
    FOR EACH ROW EXECUTE FUNCTION public.handle_new_user();
```

Sin este trigger, el usuario existe en `auth.users` pero no en `profiles`. El primer request del usuario nuevo fallaría porque no tiene profile. El trigger hace que sea atómico: usuario nuevo = profile nuevo, siempre.

**`SECURITY DEFINER`:** el trigger corre con los permisos de quien lo creó (el admin), no del usuario que hace el insert. Necesario porque un usuario normal no puede escribir en `auth.users`.

### `categories`

```sql
CREATE TABLE categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#888888',
    icon       TEXT NOT NULL DEFAULT 'tag',
    is_income  BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE(user_id, name)
);
```

**Por qué las categorías son por usuario:** cada persona categoriza distinto. Si fueran globales, todos compartirían el mismo set y no podrías personalizarlo.

**El constraint `UNIQUE(user_id, name)`:** un usuario no puede tener dos categorías con el mismo nombre. Pero dos usuarios distintos sí pueden tener una categoría llamada "comida" — la unicidad es dentro del contexto del usuario.

**`is_income`:** algunas "categorías" son ingresos (sueldo, freelance). Con este campo podés separar ingresos de gastos en los reportes sin crear una estructura más compleja.

**Cómo se crean las categorías por defecto:** cuando se crea el profile (via trigger), otro trigger podría llamar a una función que inserta categorías predeterminadas. Alternativamente, Go las inserta en el primer login. Cualquiera de los dos es válido.

### `transactions`

```sql
CREATE TABLE transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    amount          NUMERIC(12, 2) NOT NULL,
    description     TEXT NOT NULL,
    category_id     UUID REFERENCES categories(id) ON SET NULL,
    date            DATE NOT NULL,
    source          TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'csv_import')),
    import_batch_id UUID REFERENCES import_batches(id) ON DELETE SET NULL,
    raw_import_data JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Por qué `NUMERIC(12, 2)` y no `FLOAT`:** los tipos flotantes (`FLOAT`, `DOUBLE`) tienen errores de precisión. `0.1 + 0.2` en float puede dar `0.30000000000000004`. Para datos financieros, **nunca uses float**. `NUMERIC(12, 2)` almacena exactamente 12 dígitos con 2 decimales, sin error de precisión.

**Por qué `category_id` es nullable con `ON DELETE SET NULL`:** si un usuario borra una categoría, las transacciones no se borran con ella — quedan con `category_id = NULL` (sin categorizar). Sería muy malo perder el historial de transacciones porque se borró una categoría.

**`source` con CHECK constraint:** el campo solo puede ser 'manual' o 'csv_import'. El constraint lo garantiza a nivel de DB, no solo a nivel de Go. Aunque Go valide, la DB es la última línea de defensa.

**`raw_import_data JSONB`:** guarda la fila original del CSV tal como vino. Sirve para debugging (si el parser mapeó mal algo, podés ver el dato original) y para re-procesar si cambiás la lógica del parser.

**Índices importantes:**
```sql
CREATE INDEX idx_transactions_user_date ON transactions(user_id, date DESC);
CREATE INDEX idx_transactions_user_category ON transactions(user_id, category_id);
```

**Por qué estos índices:** la query más común es "dame las transacciones de este usuario en este mes". Sin índice en `(user_id, date)`, Postgres haría un full scan de toda la tabla. Con el índice, va directo a las filas relevantes. El orden `DESC` porque normalmente querés las más recientes primero.

### `budgets`

```sql
CREATE TABLE budgets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    amount      NUMERIC(12, 2) NOT NULL,
    month       CHAR(7) NOT NULL,  -- formato: '2026-06'
    UNIQUE(user_id, category_id, month)
);
```

**Por qué `CHAR(7)` para el mes y no una fecha completa:** el presupuesto es para el mes completo, no para un día específico. Guardar `'2026-06'` es más legible que guardar `'2026-06-01'` y tener que truncar siempre.

**El constraint `UNIQUE(user_id, category_id, month)`:** no tiene sentido tener dos presupuestos para "comida" en junio. Si el usuario cambia el presupuesto, hace un UPDATE, no un INSERT.

### `import_batches`

```sql
CREATE TABLE import_batches (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    filename   TEXT NOT NULL,
    row_count  INTEGER NOT NULL,
    bank       TEXT,  -- 'bbva', 'ciudad', 'apple', 'unknown'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Por qué existe esta tabla:** para poder "deshacer" un import. Si el usuario importa un CSV dos veces por accidente, puede ir al historial de imports y borrar el batch. Cuando borra un batch, todas las transacciones de ese batch se borran (via FK con `ON DELETE CASCADE` desde transactions).

```sql
-- Borrar un import entero en una query:
DELETE FROM import_batches WHERE id = $1 AND user_id = $2;
-- El CASCADE borra automáticamente todas las transactions de ese batch
```

---

## 8. Row Level Security (RLS) — por qué es crítico

### Qué es RLS

RLS es una feature de Postgres que permite definir políticas de acceso a nivel de fila. Con RLS activado, incluso si alguien hace una query sin WHERE, solo verá sus propias filas.

```sql
-- Activar RLS
ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;

-- Política: solo puedo ver mis propias transacciones
CREATE POLICY "transactions_select_own" ON transactions
    FOR SELECT
    USING (user_id = auth.uid());  -- auth.uid() es la función de Supabase que devuelve el user del JWT
```

### ¿Es necesario si Go ya filtra por user_id?

En este proyecto, Go siempre filtra por user_id extraído del JWT. Entonces técnicamente, un usuario nunca podría ver datos de otro a través de la API.

Pero RLS agrega una segunda línea de defensa:

1. **Bug en Go:** si accidentalmente escribís un endpoint sin el filtro de user_id, RLS lo atrapa.
2. **Acceso directo a la DB:** si alguien obtiene credenciales de la DB (no del JWT), RLS sigue protegiendo los datos.
3. **Buenas prácticas:** las auditorías de seguridad esperan defensa en profundidad.

**Conclusión:** activá RLS desde el día 1. El costo es cero (una línea de SQL). El beneficio es una capa extra de seguridad.

---

## 9. Import CSV — diseño del parser

### El problema

Apple Wallet, BBVA y Banco Ciudad exportan CSVs con formatos distintos:

| Banco | Columna fecha | Columna monto | Encoding |
|---|---|---|---|
| Apple Wallet (iOS 16) | "Transaction Date" | "Amount (ARS)" | UTF-8 |
| Apple Wallet (iOS 17+) | "Date" | "Amount" | UTF-8 |
| BBVA Argentina | "Fecha" | "Importe" | Latin-1 |
| Ciudad | "FECHA" | "MONTO" | UTF-8 con BOM |

### Arquitectura del parser

```go
// Interfaz común — todos los parsers implementan esto
type BankParser interface {
    Parse(rows [][]string) ([]TransactionDraft, error)
    Detect(headers []string) bool  // devuelve true si este parser puede manejar el CSV
}

// Auto-detección
func DetectParser(headers []string) BankParser {
    parsers := []BankParser{
        &BBVAParser{},
        &CiudadParser{},
        &AppleWalletParser{},
    }
    for _, p := range parsers {
        if p.Detect(headers) {
            return p
        }
    }
    return &GenericParser{}  // fallback: deja que el usuario mapee columnas manualmente
}
```

**Por qué una interfaz:** si el día de mañana agregás soporte para Santander, solo creás un nuevo `SantanderParser{}` que implementa la interfaz. No tocás el código existente. Esto es el principio Open/Closed: abierto para extensión, cerrado para modificación.

### Deduplicación

Si el usuario importa el mismo CSV dos veces, no queremos duplicados.

```go
// Antes de insertar, verificar si ya existe una transacción con mismos datos
func (r *TransactionRepo) IsDuplicate(ctx context.Context, userID string, t TransactionDraft) (bool, error) {
    var count int
    err := r.db.QueryRow(ctx, `
        SELECT COUNT(*) FROM transactions
        WHERE user_id = $1
        AND date = $2
        AND amount = $3
        AND description = $4
    `, userID, t.Date, t.Amount, t.Description).Scan(&count)
    return count > 0, err
}
```

**Por qué no confiar solo en el `import_batch_id`:** dos imports distintos pueden contener las mismas transacciones. La deduplicación por contenido (fecha + monto + descripción) es más robusta.

---

## 10. Estructura de carpetas del proyecto Go

```
finanzas-api/
├── cmd/
│   └── api/
│       └── main.go          ← punto de entrada del programa
├── internal/
│   ├── handlers/
│   │   ├── transactions.go  ← HTTP handlers para /transactions
│   │   ├── budgets.go
│   │   └── import.go
│   ├── services/
│   │   ├── transactions.go  ← lógica de negocio
│   │   └── import.go        ← lógica de parseo y deduplicación
│   ├── repository/
│   │   ├── transactions.go  ← queries SQL
│   │   └── categories.go
│   ├── middleware/
│   │   ├── auth.go          ← validación JWT
│   │   └── cors.go          ← CORS para que React pueda llamar a la API
│   └── models/
│       └── models.go        ← structs compartidos (Transaction, Budget, etc.)
├── pkg/
│   └── csvparser/
│       ├── parser.go        ← interfaz BankParser
│       ├── bbva.go
│       ├── ciudad.go
│       └── apple.go
├── go.mod                   ← dependencias del proyecto
├── go.sum                   ← checksums de dependencias
├── Dockerfile               ← para deploy en Railway
└── .env.example             ← variables de entorno (sin valores reales)
```

**Por qué `internal/`:** en Go, el directorio `internal/` es especial — su contenido solo puede ser importado desde dentro del mismo módulo. Esto impide que código externo importe tus packages internos accidentalmente. Es la forma idiomática de Go de decir "esto es privado a este proyecto".

**Por qué `pkg/` para el CSV parser:** el parser podría reutilizarse en otros proyectos (un CLI tool, por ejemplo). Al ponerlo en `pkg/`, señalás que es código reutilizable y potencialmente importable desde afuera.

**Por qué `cmd/api/main.go` y no `main.go` en la raíz:** si el día de mañana necesitás un segundo comando (un CLI de migración, un worker, un seed script), podés agregarlo en `cmd/migration/main.go` sin conflicto. Es la estructura estándar para proyectos Go que pueden tener múltiples binarios.

### Dependencias Go (go.mod)

```
github.com/jackc/pgx/v5          ← driver de Postgres. Elegido sobre database/sql porque tiene soporte nativo para tipos de Postgres (UUID, JSONB, arrays) sin casting
github.com/golang-jwt/jwt/v5     ← validación de JWTs. La librería estándar de facto para Go
github.com/go-chi/chi/v5         ← router HTTP. Más simple que Gin/Echo, más potente que net/http puro. Perfecto para APIs REST
github.com/joho/godotenv         ← cargar variables de entorno desde .env en desarrollo
```

**Por qué `chi` y no `gin`:** Gin es más popular, pero chi es más idiomático (usa `net/http` estándar, fácil de testear). Para una API relativamente simple, chi es suficiente y te obliga a entender HTTP en Go sin abstracciones excesivas.

**Por qué `pgx` y no `database/sql`:** `database/sql` es el estándar de Go pero requiere conversiones manuales para tipos Postgres como UUID y JSONB. `pgx` los maneja nativamente. Menos boilerplate, menos bugs de conversión.

---

## 11. Deploy — Vercel + Railway

### Frontend en Vercel

```
# Configuración en vercel.json (si necesitás)
{
  "rewrites": [{ "source": "/(.*)", "destination": "/index.html" }]
}
```

El rewrite es necesario porque React Router maneja las rutas del lado del cliente. Sin esto, si el usuario va directamente a `tuapp.vercel.app/dashboard`, Vercel buscaría un archivo `dashboard.html` que no existe.

**Variables de entorno en Vercel:**
```
VITE_SUPABASE_URL=https://xxxx.supabase.co
VITE_SUPABASE_ANON_KEY=eyJhbGci...
VITE_API_URL=https://finanzas-api.railway.app
```

`VITE_` es el prefijo que Vite usa para exponer variables al código del browser. Variables sin ese prefijo no son accesibles desde el frontend (son privadas al proceso de build).

### Backend Go en Railway

```dockerfile
# Dockerfile — multi-stage build para imagen pequeña
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download              # descarga dependencias (cacheado por Docker)
COPY . .
RUN go build -o api ./cmd/api   # compila el binario

FROM alpine:latest               # imagen final mucho más pequeña (sin Go toolchain)
WORKDIR /app
COPY --from=builder /app/api .
EXPOSE 8080
CMD ["./api"]
```

**Por qué multi-stage build:** la imagen del builder incluye todo el toolchain de Go (~700MB). La imagen final solo necesita el binario compilado. Con multi-stage, la imagen deployada es ~15MB en vez de 700MB.

**Variables de entorno en Railway:**
```
DATABASE_URL=postgresql://postgres...
SUPABASE_JWT_SECRET=tu-jwt-secret-de-supabase
PORT=8080
ENVIRONMENT=production
```

`SUPABASE_JWT_SECRET` se encuentra en Supabase Dashboard → Settings → API → JWT Secret. Go lo necesita para verificar la firma de los JWTs.

### CORS — por qué es necesario

El browser tiene una política de seguridad que impide que `tuapp.vercel.app` haga requests a `api.railway.app`. CORS es el mecanismo que permite o deniega esto.

```go
// middleware/cors.go
func CORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "https://tuapp.vercel.app")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
        
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Por qué no `Access-Control-Allow-Origin: *`:** el wildcard permite cualquier origen, incluyendo sitios maliciosos. En producción siempre especificá el origen exacto de tu frontend.

---

## 12. Lo que se descartó y por qué

### Supabase SDK en Go

Supabase tiene un SDK oficial para Go. No lo usamos porque:
- El SDK es una abstracción sobre la API REST de Supabase (PostgREST)
- Esa API tiene limitaciones para queries complejas (JOINs, CTEs, window functions)
- Conectarse directo con pgx a Postgres da acceso a todo el poder de SQL
- El SDK es una dependencia más que puede quedar desactualizada

**Regla general:** usa el SDK cuando te simplifica la vida sin limitaciones. Acá las limitaciones son reales.

### ORM (GORM, ent)

Los ORMs mapean structs de Go a tablas SQL automáticamente. Se descartaron porque:
- Generan SQL que no podés predecir ni optimizar
- Las queries complejas (como el resumen mensual por categoría) son más claras en SQL puro
- Aprender SQL real es más valioso para entrevistas que aprender la API de un ORM
- `pgx` con SQL escrito a mano es perfectamente manejable para este scope

### GraphQL

Hay frameworks (gqlgen) que generan una API GraphQL en Go automáticamente. Se descartó porque:
- El frontend es simple y sabe exactamente qué datos necesita — REST es suficiente
- GraphQL agrega complejidad de setup y tooling
- Para entrevistas en Mercado Libre, REST + Go bien hecho pesa más que GraphQL

### Redis para caché

Para cachear queries frecuentes. Se descartó porque:
- El volumen de datos por usuario es bajo (cientos o miles de transacciones, no millones)
- Los índices de Postgres son suficientes para este scale
- Agregar Redis es otra infraestructura que mantener
- **Regla:** no optimizés lo que no mediste que es lento

---

## 13. Roadmap técnico por fases

### Fase 1 — Supabase + schema (45 min)

- [ ] Crear proyecto en supabase.com
- [ ] Ejecutar el SQL de las tablas (ver sección 7)
- [ ] Activar RLS y crear policies
- [ ] Crear el trigger de profiles
- [ ] Instalar Supabase CLI: `npm install -g supabase`
- [ ] `supabase gen types typescript --project-id xxxx > src/types/database.ts`

### Fase 2 — Scaffold Go + Auth middleware (45 min)

- [ ] `go mod init github.com/erne/finanzas-api`
- [ ] Crear estructura de carpetas
- [ ] Implementar `middleware/auth.go` — validar JWT
- [ ] Implementar `middleware/cors.go`
- [ ] Endpoint `GET /health` para verificar que la API responde
- [ ] Conectar a Postgres con pgx
- [ ] Deploy en Railway con el Dockerfile

### Fase 3 — CRUD de transacciones (1 hora)

- [ ] `repository/transactions.go` — Insert, List, Delete
- [ ] `services/transactions.go` — validaciones
- [ ] `handlers/transactions.go` — HTTP handlers
- [ ] Rutas: `GET /transactions`, `POST /transactions`, `DELETE /transactions/:id`
- [ ] Probar con `curl` o Bruno/Insomnia

### Fase 4 — Frontend React (1 hora)

- [ ] Setup Vite + React + TypeScript
- [ ] Instalar: `@supabase/supabase-js`, `shadcn/ui`, `recharts`
- [ ] `lib/supabase.ts` — cliente singleton
- [ ] `hooks/useSession.ts` — manejo del JWT
- [ ] Páginas: Login, Dashboard, Transactions
- [ ] Deploy en Vercel

### Fase 5 — Import CSV (1 hora 30 min)

- [ ] `pkg/csvparser/` — interfaz + parsers por banco
- [ ] `handlers/import.go` — recibir el archivo, devolver preview
- [ ] `services/import.go` — deduplicación, insert en batch
- [ ] UI de import en React — drag & drop, preview, confirmar

---

## 14. Lo que aprendimos ejecutando la Fase 1 (28/06/2026)

Esta sección documenta lo que difirió entre el plan y la realidad. Es la parte más valiosa del documento — son decisiones que viviste, no que leíste.

### Supabase cambió su sistema de JWT Keys

**Lo planeado:** usar `SUPABASE_JWT_SECRET` directo desde Settings → API → JWT Secret con algoritmo HS256.

**Lo que encontramos:** Supabase migró a claves asimétricas ECC (P-256) como sistema por defecto. El dashboard ahora muestra "JWT Keys" con una "Current Key" de tipo ECC y una "Previous Key" de tipo Legacy HS256.

**Por qué importa:** `golang-jwt` configurado para HS256 (clave secreta compartida) no puede validar tokens firmados con ECC P-256 (clave pública/privada). Son algoritmos distintos — HS256 usa una sola clave para firmar y verificar, ECC usa un par.

**La decisión:** usar la Legacy HS256 (Previous Key) para este proyecto. Es compatible con `golang-jwt` sin configuración extra y sigue siendo válida mientras los tokens existentes no expiren.

**Para el futuro:** si querés usar ECC P-256, necesitás cambiar el middleware de Go para obtener la clave pública de Supabase via JWKS endpoint (`https://[project].supabase.co/.well-known/jwks.json`) en vez de usar el secret directo. Más seguro, más complejo.

---

### El connection string con caracteres especiales rompe pgx

**Lo planeado:** copiar el URI de Supabase directo al `.env`.

**Lo que encontramos:** la password generada automáticamente por Supabase contenía `#`. En una URI, `#` marca el inicio del fragmento — el parser de `pgx` interpretó todo lo que venía después como parte del host, no de la password.

**El error:**
```
invalid port ":uUiqzk6uQacTkEa" after host
```

**Las soluciones posibles:**
1. URL-encodear el caracter especial: `#` → `%23`
2. Usar formato key-value en vez de URI: `host=... port=... password=...`
3. Resetear la password por una sin caracteres especiales

**La decisión:** resetear la password a algo simple sin caracteres especiales (`v` en este caso, para desarrollo local). Para producción en Railway, usar una password alfanumérica sin símbolos.

**Regla que sale de esto:** nunca uses caracteres especiales (`#`, `@`, `!`, `$`, `%`) en passwords de DB que van en connection strings URI. En producción, Railway y Supabase te dan la opción de generar passwords seguras alfanuméricas — preferí esas.

---

### Connection method: Session Pooler, no Direct Connection

**Lo planeado:** Transaction Pooler (mencionado en el doc original).

**Lo que encontramos:** Direct Connection usa IPv6 por defecto en Supabase. Desde redes domésticas en Argentina (y la mayoría de las redes), IPv6 no está disponible. La conexión falla silenciosamente o da timeout.

**Los tres métodos y cuándo usar cada uno:**

| Método | Puerto | Cuándo usarlo |
|---|---|---|
| Direct Connection | 5432 | Servidores en la nube con IPv6, conexiones persistentes de larga duración |
| Transaction Pooler | 6543 | Serverless functions, Edge Functions, conexiones muy cortas y masivas |
| Session Pooler | 5432 | Go en Railway, cualquier servidor con IPv4, conexiones persistentes |

**La decisión:** Session Pooler es correcto para Go en Railway. Puerto 5432, IPv4, maneja bien conexiones persistentes de una API REST.

---

### Data API desactivada: el warning de Supabase es correcto pero no aplica

**Lo encontrado:** al desactivar la Data API, Supabase muestra:
> "Client libraries need Data API to query your database"

**Por qué no nos aplica:** ese warning es para proyectos donde el frontend usa `supabase-js` para hacer queries directas a la DB. En nuestra arquitectura, el frontend solo usa `supabase-js` para Auth (`signIn`, `signUp`) — que usa endpoints de Auth, no la Data API. Todas las queries de datos pasan por Go via `pgx` directo a Postgres.

**Regla:** la Data API (PostgREST) es el backend de los proyectos que no tienen backend propio. Si tenés Go, no la necesitás.

---

### Claude Code aplicó dos mejoras al SQL que no estaban en el plan

**Mejora 1 — `ENABLE ROW LEVEL SECURITY` explícito en cada tabla:**

El plan decía activar RLS via "Enable automatic RLS" en la configuración del proyecto. Claude Code lo agregó explícitamente en el SQL por dos razones:
- La configuración del proyecto puede cambiar o resetearse
- El SQL del schema debe ser autocontenido — si alguien ejecuta este SQL en otro proyecto de Supabase sin esa configuración, RLS igual queda activado
- Hace el intent explícito y auditable en el historial de Git

**Mejora 2 — `SET search_path = ''` en la función `SECURITY DEFINER`:**

```sql
CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS TRIGGER AS $$
BEGIN
    SET search_path = '';  -- esto agregó Claude Code
    INSERT INTO public.profiles (id) VALUES (NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
```

**Por qué:** una función con `SECURITY DEFINER` corre con permisos de admin. Si no fijás el `search_path`, un usuario malicioso podría crear un schema con funciones del mismo nombre que las del sistema (`gen_random_uuid`, por ejemplo) y la función privilegiada las llamaría en vez de las reales. Con `SET search_path = ''`, le decís a la función que use paths absolutos (`public.profiles`) y no busque en schemas ambiguos.

Es un ataque poco común pero documentado en la seguridad de Postgres. Cuesta cero agregarlo.

---

### Estado al final de la Fase 1

- ✅ API Go corriendo en `localhost:8080`
- ✅ Health check: `GET /health` → `{"status":"ok"}`
- ✅ Conectada a Postgres en Supabase (São Paulo)
- ✅ 5 tablas creadas: profiles, categories, import_batches, transactions, budgets
- ✅ 3 índices de performance
- ✅ RLS activado en todas las tablas con policies `own_*`
- ✅ Trigger `on_auth_user_created` para crear profiles automáticamente
- ✅ Data API desactivada, RLS automático activado
- ⏳ Pendiente: primer commit al repo, Fase 2 (endpoints de transactions)

---

## Notas finales

Este documento debe actualizarse cada vez que se toma una decisión de arquitectura que difiere de lo planeado. Las decisiones que parecen obvias hoy son las que más duelen entender en 6 meses sin documentación.

**Principios que guiaron las decisiones:**
1. **Explícito sobre implícito** — preferir código que deje en claro qué está pasando
2. **Simple hasta que no alcance** — no agregar Redis, GraphQL, o microservicios hasta que el problema los requiera
3. **La DB es la última línea de defensa** — constraints, tipos correctos, y RLS aunque Go ya valide
4. **Aprender > velocidad** — elegir el camino que enseña más aunque tarde un poco más
