{-# LANGUAGE OverloadedStrings #-}

module Smap.Query.Parser (parseQuery) where

import Data.Text (Text)
import qualified Data.Text as T
import Data.Void
import Text.Megaparsec
import Text.Megaparsec.Char
import qualified Text.Megaparsec.Char.Lexer as L
import Control.Monad.Combinators.Expr
import Smap.Query.AST

type Parser = Parsec Void Text

sc :: Parser ()
sc = L.space
  space1
  (L.skipLineComment "--")
  (L.skipBlockComment "/*" "*/")

lexeme :: Parser a -> Parser a
lexeme = L.lexeme sc

symbol :: Text -> Parser Text
symbol = L.symbol sc

parens :: Parser a -> Parser a
parens = between (symbol "(") (symbol ")")

-- Keywords
keyword :: Text -> Parser ()
keyword w = (lexeme . try) (string' w *> notFollowedBy alphaNumChar)

-- Parsers for AST components
pQuery :: Parser Query
pQuery = Select <$> pSelect
      <|> Set <$> pSet
      <|> Delete <$> pDelete

pSelect :: Parser SelectQuery
pSelect = do
    keyword "SELECT"
    fields <- pFields
    whereClause <- optional $ keyword "WHERE" *> pWhere
    limit <- optional $ keyword "LIMIT" *> lexeme L.decimal
    streamLimit <- optional $ keyword "STREAMLIMIT" *> lexeme L.decimal
    return $ SelectQuery fields whereClause limit streamLimit

pFields :: Parser [Field]
pFields = (keyword "*" *> return [AllFields])
       <|> pField `sepBy` symbol ","

pField :: Parser Field
pField = (keyword "uuid" *> return UuidField)
      <|> (keyword "path" *> return PathField)
      <|> (keyword "data" *> (DataField <$> pDataRange))
      <|> MetadataField <$> pIdentifier

pDataRange :: Parser DataRange
pDataRange = pInRange <|> pAfterBefore <|> return (DataRange Nothing Nothing Nothing)
  where
    pInRange = do
        keyword "IN"
        _ <- symbol "("
        start <- pTimeExpr
        _ <- symbol ","
        end <- pTimeExpr
        _ <- symbol ")"
        return $ DataRange (Just start) (Just end) Nothing
    pAfterBefore = do
        start <- optional $ keyword "AFTER" *> pTimeExpr
        end <- optional $ keyword "BEFORE" *> pTimeExpr
        return $ DataRange start end Nothing

pTimeExpr :: Parser TimeExpr
pTimeExpr = do
    base <- pTimeTerm
    pRelative base <|> return base
  where
    pRelative :: TimeExpr -> Parser TimeExpr
    pRelative base = do
        op <- (symbol "-" *> return (-1 :: Integer)) <|> (symbol "+" *> return 1)
        diff <- pDiffTime
        let adjustedDiff = if op == -1 then negateDiff diff else diff
        let next = Relative base adjustedDiff
        pRelative next <|> return next
    negateDiff (DiffTime v u) = DiffTime (-v) u

pTimeTerm :: Parser TimeExpr
pTimeTerm = parens pTimeExpr
         <|> (keyword "now" *> return Now)
         <|> (AbsolutePosix <$> try L.float)
         <|> (AbsolutePosix . fromIntegral <$> try (L.decimal :: Parser Integer))
         <|> (AbsoluteIso <$> pStringLiteral)

pDiffTime :: Parser DiffTime
pDiffTime = do
    val <- lexeme L.decimal
    unit <- pTimeUnit
    return $ DiffTime val unit

pTimeUnit :: Parser TimeUnit
pTimeUnit = (keyword "s" <|> keyword "second" <|> keyword "seconds") *> return Second
         <|> (keyword "m" <|> keyword "minute" <|> keyword "minutes") *> return Minute
         <|> (keyword "h" <|> keyword "hour" <|> keyword "hours") *> return Hour
         <|> (keyword "d" <|> keyword "day" <|> keyword "days") *> return Day
         <|> (keyword "w" <|> keyword "week" <|> keyword "weeks") *> return Week

pWhere :: Parser WhereClause
pWhere = makeExprParser pWhereTerm whereOperators

pWhereTerm :: Parser WhereClause
pWhereTerm = parens pWhere
          <|> try pEquals
          <|> try pHas
          <|> try pLike

pHas :: Parser WhereClause
pHas = keyword "has" *> (Has <$> pIdentifier)

pEquals :: Parser WhereClause
pEquals = do
    key <- pIdentifier
    _ <- symbol "="
    val <- pStringLiteral
    return $ Equals key val

pLike :: Parser WhereClause
pLike = do
    key <- pIdentifier
    keyword "like"
    val <- pStringLiteral
    return $ Like key val

whereOperators :: [[Operator Parser WhereClause]]
whereOperators =
  [ [ Prefix (keyword "NOT" *> return Not) ]
  , [ InfixL (keyword "AND" *> return And) ]
  , [ InfixL (keyword "OR" *> return Or) ]
  ]

pIdentifier :: Parser Text
pIdentifier = lexeme $ T.pack <$> some (alphaNumChar <|> char '/' <|> char '_')

pStringLiteral :: Parser Text
pStringLiteral = lexeme $ char '\'' *> (T.pack <$> manyTill L.charLiteral (char '\''))

-- Placeholders
pSet :: Parser SetQuery
pSet = keyword "SET" *> return SetQuery

pDelete :: Parser DeleteQuery
pDelete = keyword "DELETE" *> return DeleteQuery

-- Top-level function
parseQuery :: Text -> Either (ParseErrorBundle Text Void) Query
parseQuery = parse (sc *> pQuery <* eof) ""
