package main

import (
	"log/slog"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"tea-platform/pkg/response"
)

// writeGRPCError транслирует gRPC-ошибку в HTTP-ответ. Клиентские ошибки
// (InvalidArgument/NotFound) отдаём как есть, серверные — обобщаем и логируем.
func writeGRPCError(w http.ResponseWriter, log *slog.Logger, op string, err error) {
	st := status.Convert(err)
	switch st.Code() {
	case codes.InvalidArgument:
		response.Error(w, http.StatusBadRequest, st.Message())
	case codes.NotFound:
		response.Error(w, http.StatusNotFound, st.Message())
	case codes.FailedPrecondition:
		response.Error(w, http.StatusConflict, st.Message())
	case codes.Unavailable:
		log.Error("upstream unavailable", "op", op, "error", err)
		response.Error(w, http.StatusBadGateway, "Service unavailable")
	default:
		log.Error("upstream error", "op", op, "error", err)
		response.Error(w, http.StatusInternalServerError, "Internal error")
	}
}
