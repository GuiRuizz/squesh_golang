package handler

import (
	"net/http"
	"strings"

	"squesh_golang/internal/domain"
	"squesh_golang/internal/dto"
	"squesh_golang/internal/service"
	"squesh_golang/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{DB: db}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	// Obter do contexto do Gin
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	// Type assertion direto para uuid.UUID
	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	// Agora você já tem a variável 'userID' do tipo uuid.UUID para usar nas buscas do GORM!

	var user domain.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	c.JSON(http.StatusOK, h.profileResponse(&user))
}

// userFromContext devolve o usuário logado, já com o perfil carregado.
func (h *UserHandler) userFromContext(c *gin.Context) (*domain.User, bool) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return nil, false
	}

	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return nil, false
	}

	var user domain.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return nil, false
	}
	return &user, true
}

// profileResponse monta o DTO do perfil. A ofensiva é DERIVADA dos dias
// completos (alimentação + treino 100%), não da coluna: aqui ela é recalculada
// e, de quebra, o progresso parcial dos dias passados é descartado.
func (h *UserHandler) profileResponse(user *domain.User) dto.UserProfileResponseDTO {
	if state, err := service.SyncUserDays(h.DB, user.ID); err == nil {
		user.StreakCount = state.Streak
	}

	prefs := user.Preferences
	return dto.UserProfileResponseDTO{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		AvatarURL: user.AvatarURL,
		Bio:       user.Bio,
		Streak:    user.StreakCount,
		Points:    user.Points,
		Preferences: dto.UserPreferencesResponseDTO{
			PushEnabled:      prefs.Push(),
			WorkoutReminders: boolOrDefault(prefs.WorkoutReminders),
			ShopNews:         boolOrDefault(prefs.ShopNews),
			SocialAlerts:     boolOrDefault(prefs.SocialAlerts),
		},
		CreatedAt: user.CreatedAt,
	}
}

func boolOrDefault(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

// GetRanking retorna a lista de usuários ordenada pelo maior streak
func (h *UserHandler) GetRanking(c *gin.Context) {
	var users []domain.User

	// Busca todos os usuários do banco
	if err := h.DB.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar usuários para o ranking"})
		return
	}

	// Lista para armazenar o ranking calculado
	var ranking []dto.UserRankingDTO

	for _, user := range users {
		// Mesma ofensiva do perfil: derivada dos dias completos de cada usuário.
		streak := user.StreakCount
		if state, err := service.SyncUserDays(h.DB, user.ID); err == nil {
			streak = state.Streak
		}

		// Opcional: Descomente a linha abaixo se quiser exibir apenas usuários com streak > 0
		// if streak == 0 { continue }

		ranking = append(ranking, dto.UserRankingDTO{
			ID:        user.ID,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			Streak:    streak,
		})
	}

	// Ordena a lista do maior para o menor streak
	// Em caso de empate no streak, mantemos a ordem atual ou podemos ordenar por nome
	for i := 0; i < len(ranking); i++ {
		for j := i + 1; j < len(ranking); j++ {
			if ranking[j].Streak > ranking[i].Streak {
				ranking[i], ranking[j] = ranking[j], ranking[i]
			}
		}
	}

	// Atribui as posições (1º, 2º, 3º...) após a ordenação
	for i := range ranking {
		ranking[i].Position = i + 1
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  ranking,
		"total": len(ranking),
	})
}

// UpdateProfile atualiza os dados básicos do usuário logado (Nome, Bio e Avatar).
// Atualização PARCIAL: só os campos presentes no corpo são gravados, para o app
// poder editar um campo por vez sem apagar os outros.
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	user, ok := h.userFromContext(c)
	if !ok {
		return
	}

	var input dto.UpdateProfileDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Nome não pode ficar vazio"})
			return
		}
		user.Name = name
	}
	if input.Bio != nil {
		user.Bio = strings.TrimSpace(*input.Bio)
	}
	if input.AvatarURL != nil {
		user.AvatarURL = strings.TrimSpace(*input.AvatarURL)
	}

	if err := h.DB.Model(user).Select("name", "bio", "avatar_url", "updated_at").Updates(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar dados do usuário"})
		return
	}

	c.JSON(http.StatusOK, h.profileResponse(user))
}

// UpdatePreferences salva as preferências de notificação (PUT /users/me/preferences).
// Parcial pelos mesmos motivos do perfil: só o que vier no corpo muda.
func (h *UserHandler) UpdatePreferences(c *gin.Context) {
	user, ok := h.userFromContext(c)
	if !ok {
		return
	}

	var input dto.UpdatePreferencesDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prefs := user.Preferences
	if prefs.PushEnabled == nil || prefs.WorkoutReminders == nil || prefs.ShopNews == nil || prefs.SocialAlerts == nil {
		// Usuário sem preferências gravadas ainda: começa do padrão (tudo ligado)
		// para desligar um switch não mantiver os outros como "nunca configurado".
		prefs = domain.DefaultPreferences()
	}
	if input.PushEnabled != nil {
		prefs.PushEnabled = input.PushEnabled
	}
	if input.WorkoutReminders != nil {
		prefs.WorkoutReminders = input.WorkoutReminders
	}
	if input.ShopNews != nil {
		prefs.ShopNews = input.ShopNews
	}
	if input.SocialAlerts != nil {
		prefs.SocialAlerts = input.SocialAlerts
	}

	user.Preferences = prefs
	if err := h.DB.Model(user).Select("preferences", "updated_at").Updates(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao salvar preferências"})
		return
	}

	c.JSON(http.StatusOK, h.profileResponse(user))
}

// UpdatePassword realiza a troca de senha com verificação da senha atual
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	var input dto.UpdatePasswordDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user domain.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	// Valida se a senha atual informada bate com a salva no banco
	// (Assumindo que você usa utils.CheckPasswordHash com bcrypt)
	if !utils.CheckPasswordHash(input.CurrentPassword, user.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A senha atual está incorreta"})
		return
	}

	// Gera o hash da nova senha
	hashedPassword, err := utils.HashPassword(input.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao processar nova senha"})
		return
	}

	user.Password = hashedPassword

	if err := h.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao salvar nova senha"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Senha alterada com sucesso"})
}

// GetStreak retorna apenas as informações referentes à ofensiva (streak) do usuário logado
func (h *UserHandler) GetStreak(c *gin.Context) {
	userIDCtx, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Não autorizado"})
		return
	}

	userID, ok := userIDCtx.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ID de usuário inválido no contexto"})
		return
	}

	// Valida se o usuário existe no banco
	var user domain.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	// A ofensiva é derivada dos dias em que o usuário fechou 100% de um dia de
	// alimentação E 100% de uma sessão de treino.
	state, err := service.SyncUserDays(h.DB, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao calcular a ofensiva"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": user.ID,
		"streak":  state.Streak,
	})
}
