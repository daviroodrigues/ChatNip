-- Cuidado: Isso irá cortar dados se eles forem maiores que 255 caracteres ao reverter
ALTER TABLE whatsapp_sessions ALTER COLUMN proxy_url TYPE VARCHAR(255);
