CREATE TABLE IF NOT EXISTS whatsapp_sessions (
    -- O ID é a chave primária, usada no ON CONFLICT do código Go
    id VARCHAR(255) PRIMARY KEY,

-- Tenant ID é obrigatório para identificar o cliente
tenant_id VARCHAR(255) NOT NULL,

-- JID (WhatsApp ID) pode ser atualizado depois, mas geralmente existe
jid VARCHAR(255),

-- Importante: O Go não envia este campo no INSERT, então o DEFAULT é obrigatório
created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP );
