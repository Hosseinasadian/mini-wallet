package main

func main() {

	//
	//r := gin.Default()
	//
	//r.GET("/ping", PingHandler)
	//r.POST("/register", RegisterHandler)
	//r.POST("/login", LoginHandler)
	//r.POST("/refresh-token", RefreshTokenHandler)
	//
	//ur := r.Group("/user")
	//ur.Use(AuthMiddleware())
	//
	//ur.GET("/information", InformationHandler)
	//
	//if err := r.Run(":8080"); err != nil {
	//	log.Fatal(err)
	//}
}

// params

/*type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RegisterResponse struct {
	Message      string  `json:"message"`
	AccessToken  *string `json:"user_access_token"`
	RefreshToken *string `json:"refresh_token"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LoginResponse struct {
	AccessToken  string `json:"user_access_token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}
type RefreshTokenResponse struct {
	AccessToken  string `json:"user_access_token"`
	RefreshToken string `json:"refresh_token"`
}

type User struct {
	ID           int64     `json:"id" db:"id"`
	Email        string    `json:"email"  db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type RefreshToken struct {
	ID        int64     `json:"-" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	Revoked   bool      `json:"revoked" db:"revoked"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"-" db:"created_at"`
}

// http handlers

func PingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func RegisterHandler(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "invalid request",
		})
		return
	}

	if len(req.Password) < 8 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"message": "password too short",
		})
		return
	}

	if len(req.Password) > 72 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"message": "password too long",
		})
		return
	}

	emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
	if !emailRegex.MatchString(req.Email) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"message": "invalid email",
		})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to process password",
		})
		return
	}

	var accountId int64
	accountId, err = CreateUserByEmailAndPassword(c.Request.Context(), req.Email, string(hashedPassword))
	if err != nil {
		var mysqlErr *config.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"message": "email already exists",
			})
			return
		}

		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to create user",
		})
		return
	}

	var accessToken string
	accessToken, err = GenerateToken(accountId)
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusCreated, RegisterResponse{
			Message: "account created but login failed, please login manually",
		})
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusCreated, gin.H{
			"message": "account created but login failed, please login manually",
		})
		return
	}

	expireRefreshToken := time.Now().Add(time.Hour * 24 * 7)
	newToken := hashToken(refreshToken)
	err = CreateRefreshToken(c.Request.Context(), accountId, newToken, expireRefreshToken, nil)
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusCreated, gin.H{
			"message": "account created but login failed, please login manually",
		})
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		Message:      "account successfully created",
		AccessToken:  &accessToken,
		RefreshToken: &refreshToken,
	})
}

func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "invalid request",
		})
		return
	}

	if req.Email == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"message": "email required",
		})
		return
	}

	if req.Password == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"message": "password is required",
		})
		return
	}

	user, err := GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "username or password is incorrect",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to get user",
		})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "username or password is incorrect",
		})
		return
	}

	accessToken, err := GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to generate access token",
		})
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to generate refresh token",
		})
		return
	}

	expireRefreshToken := time.Now().Add(time.Hour * 24 * 7)
	newToken := hashToken(refreshToken)
	err = CreateRefreshToken(c.Request.Context(), user.ID, newToken, expireRefreshToken, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to create refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

func InformationHandler(c *gin.Context) {
	accountId, ok := c.Get("account_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("your id is %d", accountId.(int64)),
	})
}

func RefreshTokenHandler(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}

	refreshToken, err := VerifyRefreshToken(c.Request.Context(), hashToken(parts[1]))
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}

	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to generate refresh token",
		})
		return
	}

	expireRefreshToken := time.Now().Add(time.Hour * 24 * 7)
	oldToken := refreshToken.Token
	newToken := hashToken(newRefreshToken)
	err = CreateRefreshToken(c.Request.Context(), refreshToken.UserID, newToken, expireRefreshToken, &oldToken)
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to create refresh token",
		})
		return
	}

	accessToken, err := GenerateToken(refreshToken.UserID)
	if err != nil {
		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed to generate access token",
		})
		return
	}

	c.JSON(http.StatusOK, RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// jwt token

type Claims struct {
	jwt.RegisteredClaims
	AccountID int64 `json:"account_id"`
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		tokenClaims, err := VerifyToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		c.Set("account_id", tokenClaims.AccountID)
		c.Next()
	}
}

func VerifyToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil {
		return nil, err
	}

	tokenClaims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, err
	}

	return tokenClaims, nil
}

func GenerateToken(accountID int64) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		jwt.RegisteredClaims{
			Issuer:    "mini-wallet",
			Subject:   "access",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		accountID,
	})

	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(bytes)
	return token, nil
}

// DB

func CreateUserByEmailAndPassword(ctx context.Context, email, password string) (int64, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, "INSERT INTO users (email, password_hash) VALUES (?, ?)", email, password)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return id, nil
}

func GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := db.GetContext(ctx, &user, "SELECT * FROM users WHERE email=?", email)
	if err != nil {
		return user, err
	}
	return user, nil
}

func CreateRefreshToken(ctx context.Context, userId int64, token string, expiresAt time.Time, oldToken *string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if oldToken != nil {
		_, err = tx.ExecContext(ctx, "UPDATE refresh_tokens set revoked=? WHERE token=?", true, *oldToken)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO refresh_tokens (user_id, token,expires_at) VALUES (?, ?, ?)", userId, token, expiresAt)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}

func VerifyRefreshToken(ctx context.Context, refreshToken string) (*RefreshToken, error) {
	var token RefreshToken
	err := db.GetContext(ctx, &token, "SELECT * FROM refresh_tokens WHERE token=?", refreshToken)
	if err != nil {
		return nil, err
	}

	if token.Revoked || token.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token is invalid")
	}

	return &token, nil
}
*/
