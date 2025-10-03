# Coding Standards

Code style guidelines for MyGamesAnywhere. These standards ensure consistency, maintainability, and clarity.

## Core Principles

### 1. Clarity First
- Always ensure 100% clarity and confidence
- If unsure, ask rather than guess
- Never proceed with ambiguity
- Code should be self-documenting

### 2. Object-Oriented Design (Where Possible)
- Prefer classes over standalone functions for stateful logic
- Use interfaces for contracts
- Favor composition over inheritance
- Encapsulate related data and behavior

### 3. RAII (Resource Acquisition Is Initialization)
- Acquire resources in constructors/initialization
- Release resources in destructors/cleanup methods
- Use `defer` in Go, try-finally patterns in TypeScript
- Ensures resource cleanup even during errors

### 4. Fail-Fast Policy
- Use detailed errors with context
- NO silent fallbacks or swallowing errors
- Only fallback when explicitly required
- Make bugs obvious immediately

### 5. Minimize Code Duplication
- Extract shared logic into functions/classes
- Use base classes or composition
- Aggressively refactor duplicate code
- DRY (Don't Repeat Yourself) principle

### 6. Code Paragraphs
- Place blank lines between logical code blocks
- Add comments explaining each paragraph's goal
- Makes debugging easier
- Improves readability

---

## TypeScript/React Standards

### File Naming

```
components/
├── Button.tsx              # PascalCase for components
├── Button.test.tsx         # .test.tsx for tests
└── Button.module.css       # .module.css for CSS modules

utils/
├── validation.ts           # camelCase for utilities
└── validation.test.ts

hooks/
├── useAuth.ts              # camelCase with 'use' prefix
└── useAuth.test.ts
```

### Component Structure

```typescript
// Good: Class-based for stateful components
export class GameCard extends Component<GameCardProps, GameCardState> {
  constructor(props: GameCardProps) {
    super(props);

    // Initialize state
    this.state = {
      isHovered: false,
      isLoading: false
    };
  }

  // Event handlers
  private handleMouseEnter = (): void => {
    this.setState({ isHovered: true });
  };

  // Render
  render(): ReactNode {
    const { game } = this.props;
    const { isHovered } = this.state;

    return (
      <div
        className="game-card"
        onMouseEnter={this.handleMouseEnter}
      >
        <h3>{game.title}</h3>
      </div>
    );
  }
}

// Also Good: Functional for simple/presentational components
export function Button({ children, onClick }: ButtonProps): JSX.Element {
  return (
    <button onClick={onClick} className="btn">
      {children}
    </button>
  );
}
```

### Naming Conventions

```typescript
// Interfaces/Types: PascalCase
interface User {
  id: string;
  email: string;
}

type GameStatus = 'installed' | 'downloading' | 'pending';

// Classes: PascalCase
class AuthService {
  // ...
}

// Functions: camelCase
function validateEmail(email: string): boolean {
  // ...
}

// Constants: SCREAMING_SNAKE_CASE
const API_BASE_URL = 'http://localhost:8080';
const MAX_RETRIES = 3;

// Variables: camelCase
const user = await getUser();
const isValid = validateEmail(email);

// Private members: prefix with _
class GameService {
  private _cache: Map<string, Game>;

  constructor() {
    this._cache = new Map();
  }
}
```

### Code Paragraphs

```typescript
async function installGame(gameId: string, destination: string): Promise<void> {
  // Validate game exists and get download info
  const game = await this.gameService.getById(gameId);
  if (!game) {
    throw new DetailedError(
      'GAME_NOT_FOUND',
      `Game ${gameId} not found`,
      { gameId }
    );
  }

  // Create destination directory
  await fs.mkdir(destination, { recursive: true });

  // Download game file
  const tempFile = path.join(destination, 'download.tmp');
  await this.downloadService.download(game.downloadUrl, tempFile);

  // Extract archive
  await this.extractService.extract(tempFile, destination);

  // Clean up temporary file
  await fs.unlink(tempFile);

  // Update database
  await this.gameService.markInstalled(gameId, destination);
}
```

### Error Handling

```typescript
// Define error class
export class DetailedError extends Error {
  constructor(
    public code: string,
    message: string,
    public context?: Record<string, unknown>,
    public cause?: Error
  ) {
    super(message);
    this.name = 'DetailedError';
  }
}

// Throw with context
if (!user) {
  throw new DetailedError(
    'USER_NOT_FOUND',
    `User with ID ${userId} not found`,
    {
      userId,
      timestamp: new Date().toISOString(),
      requestId: req.id
    }
  );
}

// Catch and re-throw with context
try {
  await api.fetchUser(userId);
} catch (error) {
  throw new DetailedError(
    'USER_FETCH_FAILED',
    'Failed to fetch user from API',
    { userId },
    error as Error
  );
}

// ❌ BAD: Silent failure
try {
  await api.fetchUser(userId);
} catch {
  return null;  // Lost error information!
}
```

### Zustand Store Pattern

```typescript
// Define state interface
interface AuthState {
  // State
  user: User | null;
  token: string | null;
  isLoading: boolean;
  error: string | null;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
  refreshToken: () => Promise<void>;
}

// Create store
export const useAuthStore = create<AuthState>((set, get) => ({
  // Initial state
  user: null,
  token: null,
  isLoading: false,
  error: null,

  // Actions
  login: async (email: string, password: string) => {
    // Set loading state
    set({ isLoading: true, error: null });

    try {
      // Call API
      const response = await authApi.login(email, password);

      // Update state
      set({
        user: response.user,
        token: response.token,
        isLoading: false
      });

      // Store token
      localStorage.setItem('token', response.token);
    } catch (error) {
      // Handle error
      set({
        error: (error as Error).message,
        isLoading: false
      });

      throw error;  // Re-throw for component handling
    }
  },

  logout: () => {
    // Clear state
    set({ user: null, token: null });

    // Clear storage
    localStorage.removeItem('token');
  },

  refreshToken: async () => {
    const currentToken = get().token;
    if (!currentToken) {
      throw new DetailedError('NO_TOKEN', 'No token to refresh');
    }

    // Refresh token logic
    // ...
  }
}));
```

### Testing Patterns

```typescript
// Component test
describe('LoginForm', () => {
  it('should submit form with valid credentials', async () => {
    // Arrange
    const mockLogin = vi.fn();
    render(<LoginForm onLogin={mockLogin} />);

    // Act
    await userEvent.type(screen.getByLabelText('Email'), 'test@example.com');
    await userEvent.type(screen.getByLabelText('Password'), 'password123');
    await userEvent.click(screen.getByRole('button', { name: 'Login' }));

    // Assert
    expect(mockLogin).toHaveBeenCalledWith('test@example.com', 'password123');
  });

  it('should display error for invalid email', async () => {
    // Arrange
    render(<LoginForm />);

    // Act
    await userEvent.type(screen.getByLabelText('Email'), 'invalid-email');
    await userEvent.click(screen.getByRole('button', { name: 'Login' }));

    // Assert
    expect(screen.getByText('Invalid email format')).toBeInTheDocument();
  });
});

// Store test
describe('useAuthStore', () => {
  beforeEach(() => {
    // Reset store
    useAuthStore.setState({
      user: null,
      token: null,
      isLoading: false,
      error: null
    });
  });

  it('should login successfully', async () => {
    // Arrange
    const mockResponse = {
      user: { id: '1', email: 'test@example.com' },
      token: 'mock-token'
    };
    vi.spyOn(authApi, 'login').mockResolvedValue(mockResponse);

    // Act
    await useAuthStore.getState().login('test@example.com', 'password');

    // Assert
    expect(useAuthStore.getState().user).toEqual(mockResponse.user);
    expect(useAuthStore.getState().token).toBe('mock-token');
  });
});
```

---

## Go Standards

### File Naming

```
internal/
├── services/
│   ├── auth_service.go         # snake_case
│   └── auth_service_test.go    # _test.go suffix
├── handlers/
│   └── auth_handler.go
└── models/
    └── user.go
```

### Package Structure

```go
// Package comment (required)
// Package services provides business logic for the application.
package services

import (
    // Standard library
    "context"
    "fmt"
    "time"

    // External packages
    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"

    // Internal packages
    "github.com/GreenFuze/MyGamesAnywhere/server/internal/models"
)
```

### Naming Conventions

```go
// Types: PascalCase (exported) or camelCase (unexported)
type AuthService struct {  // Exported
    db *sql.DB
    jwtService *JWTService
}

type authConfig struct {  // Unexported
    secret string
}

// Functions: PascalCase (exported) or camelCase (unexported)
func NewAuthService(db *sql.DB) *AuthService {  // Exported
    return &AuthService{db: db}
}

func (s *AuthService) validatePassword(password string) error {  // Unexported
    // ...
}

// Constants: PascalCase or SCREAMING_SNAKE_CASE
const (
    MaxRetries = 3
    DEFAULT_TIMEOUT = 30 * time.Second
)

// Variables: camelCase (or PascalCase if exported)
var ErrUserNotFound = errors.New("user not found")  // Exported error
```

### OOP Pattern (Struct Methods)

```go
// Define struct
type GameService struct {
    db      *sql.DB
    cache   *Cache
    logger  *Logger
}

// Constructor (factory function)
func NewGameService(db *sql.DB, cache *Cache, logger *Logger) *GameService {
    return &GameService{
        db:     db,
        cache:  cache,
        logger: logger,
    }
}

// Methods
func (s *GameService) GetLibrary(ctx context.Context, userID string) ([]Game, error) {
    // Check cache first
    cached, err := s.cache.Get(ctx, "library:"+userID)
    if err == nil && cached != nil {
        return cached, nil
    }

    // Fetch from database
    games, err := s.db.GetLibrary(ctx, userID)
    if err != nil {
        return nil, &DetailedError{
            Code:    "LIBRARY_FETCH_FAILED",
            Message: "Failed to fetch library from database",
            Context: map[string]interface{}{"userID": userID},
            Cause:   err,
        }
    }

    // Update cache
    s.cache.Set(ctx, "library:"+userID, games, 5*time.Minute)

    return games, nil
}
```

### RAII with defer

```go
func ProcessGame(gameID string) error {
    // Acquire file lock
    lock, err := acquireLock(gameID)
    if err != nil {
        return err
    }
    defer releaseLock(lock)  // RAII: always released

    // Open file
    file, err := os.Open("game.dat")
    if err != nil {
        return err
    }
    defer file.Close()  // RAII: always closed

    // Begin transaction
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer func() {
        if p := recover(); p != nil {
            tx.Rollback()
            panic(p)
        } else if err != nil {
            tx.Rollback()
        } else {
            err = tx.Commit()
        }
    }()

    // Use resources
    err = processFile(tx, file)
    return err
}  // All resources automatically released here
```

### Error Handling

```go
// Define error type
type DetailedError struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Context map[string]interface{} `json:"context,omitempty"`
    Cause   error                  `json:"-"`
}

func (e *DetailedError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DetailedError) Unwrap() error {
    return e.Cause
}

// Usage
func (s *AuthService) GetUser(ctx context.Context, userID string) (*User, error) {
    // Query database
    user, err := s.db.GetUserByID(ctx, userID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, &DetailedError{
                Code:    "USER_NOT_FOUND",
                Message: fmt.Sprintf("User %s not found", userID),
                Context: map[string]interface{}{
                    "userID": userID,
                },
            }
        }

        return nil, &DetailedError{
            Code:    "DATABASE_ERROR",
            Message: "Failed to fetch user",
            Context: map[string]interface{}{
                "userID": userID,
            },
            Cause: err,
        }
    }

    return user, nil
}

// ❌ BAD: Silent failure
func GetUser(userID string) *User {
    user, err := db.GetUser(userID)
    if err != nil {
        return nil  // Lost error information!
    }
    return user
}
```

### Code Paragraphs

```go
func (s *LibraryService) CreateLibrary(ctx context.Context, userID, name string) (*Library, error) {
    // Validate input
    if name == "" {
        return nil, &DetailedError{
            Code:    "VALIDATION_ERROR",
            Message: "Library name is required",
        }
    }

    // Check if default library needs to be set
    existingLibs, err := s.db.GetUserLibraries(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to check existing libraries: %w", err)
    }
    isDefault := len(existingLibs) == 0

    // Create library
    library := &Library{
        ID:        uuid.New().String(),
        UserID:    userID,
        Name:      name,
        IsDefault: isDefault,
        CreatedAt: time.Now(),
    }

    // Save to database
    err = s.db.CreateLibrary(ctx, library)
    if err != nil {
        return nil, &DetailedError{
            Code:    "LIBRARY_CREATE_FAILED",
            Message: "Failed to create library",
            Context: map[string]interface{}{
                "userID": userID,
                "name":   name,
            },
            Cause: err,
        }
    }

    return library, nil
}
```

### Testing Patterns

```go
func TestAuthService_Login(t *testing.T) {
    // Arrange
    db := setupTestDB(t)
    defer db.Close()

    service := NewAuthService(db, &mockJWTService{})

    user := &User{
        Email:        "test@example.com",
        Username:     "testuser",
        PasswordHash: hashPassword("password123"),
    }
    createUser(t, db, user)

    // Act
    result, err := service.Login(context.Background(), "test@example.com", "password123")

    // Assert
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }

    if result.User.Email != user.Email {
        t.Errorf("expected email %s, got %s", user.Email, result.User.Email)
    }
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
    // Arrange
    db := setupTestDB(t)
    defer db.Close()

    service := NewAuthService(db, &mockJWTService{})

    // Act
    _, err := service.Login(context.Background(), "invalid@example.com", "wrong")

    // Assert
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    var detailedErr *DetailedError
    if !errors.As(err, &detailedErr) {
        t.Fatalf("expected DetailedError, got %T", err)
    }

    if detailedErr.Code != "INVALID_CREDENTIALS" {
        t.Errorf("expected code INVALID_CREDENTIALS, got %s", detailedErr.Code)
    }
}
```

---

## SQL Standards

### File Organization

```sql
-- queries/users.sql

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (
    email,
    username,
    password_hash
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: UpdateUser :exec
UPDATE users
SET
    email = $1,
    username = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $3;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;
```

### Migration File Naming

```
migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_create_libraries.up.sql
└── 000002_create_libraries.down.sql
```

### Migration Structure

```sql
-- migrations/000001_create_users.up.sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);

-- migrations/000001_create_users.down.sql
DROP TABLE IF EXISTS users;
```

---

## Comments

### When to Comment

**DO comment:**
- Why (not what)
- Complex algorithms
- Non-obvious decisions
- Code paragraphs (goal of each section)
- Public APIs (godoc/JSDoc)

**DON'T comment:**
- Obvious code
- What the code does (code should be self-explanatory)
- Outdated information

### Comment Style

**TypeScript:**
```typescript
/**
 * Validates user email and checks if it's available.
 *
 * @param email - Email address to validate
 * @returns true if email is valid and available
 * @throws {ValidationError} if email format is invalid
 */
export function validateEmail(email: string): boolean {
  // Check format using RFC 5322 regex
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(email)) {
    throw new ValidationError('Invalid email format');
  }

  // Additional validation logic
  return true;
}

// ❌ BAD: Obvious comment
// Set x to 5
const x = 5;

// ✅ GOOD: Explains why
// Default timeout increased to 30s to accommodate slow networks
const DEFAULT_TIMEOUT = 30000;
```

**Go:**
```go
// AuthService handles user authentication and authorization.
// It manages user sessions, password verification, and JWT token generation.
type AuthService struct {
    db         *sql.DB
    jwtService *JWTService
}

// Login authenticates a user with email and password.
// Returns user data and JWT token on success.
func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
    // Fetch user from database
    user, err := s.db.GetUserByEmail(ctx, email)
    if err != nil {
        return nil, err
    }

    // Verify password using bcrypt
    // bcrypt is intentionally slow to prevent brute force attacks
    err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
    if err != nil {
        return nil, ErrInvalidCredentials
    }

    // Generate JWT token
    token, err := s.jwtService.GenerateToken(user.ID)
    if err != nil {
        return nil, err
    }

    return &LoginResult{User: user, Token: token}, nil
}
```

---

## Formatting

### TypeScript

Use Prettier with default settings:

```json
// .prettierrc
{
  "semi": true,
  "trailingComma": "es5",
  "singleQuote": true,
  "printWidth": 100,
  "tabWidth": 2
}
```

### Go

Use `gofmt` (built-in):

```bash
# Format all files
gofmt -w .
```

---

## Summary

**Key Principles:**
1. ✅ Clarity first
2. ✅ OOP where appropriate
3. ✅ RAII for resource management
4. ✅ Fail-fast with detailed errors
5. ✅ Minimize duplication
6. ✅ Code paragraphs with comments

**Naming:**
- TypeScript: PascalCase (components/types), camelCase (functions/variables)
- Go: PascalCase (exported), camelCase (unexported)

**Testing:**
- Arrange-Act-Assert pattern
- Clear test names
- Mock external dependencies

**Errors:**
- Always provide context
- Never swallow errors
- Use DetailedError class/struct

Follow these standards for consistent, maintainable code!
